package roxyresolver

import (
	"errors"
	"fmt"
	"math/rand"
	"sync"

	multierror "github.com/hashicorp/go-multierror"
	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/connectivity"

	"github.com/chronos-tachyon/roxy/lib/syncrand"
)

func init() {
	balancer.Register(NewATCBalancerBuilder())
}

func NewATCBalancerBuilder() balancer.Builder {
	return atcBalancerBuilder{}
}

// type atcBalancerBuilder {{{

type atcBalancerBuilder struct{}

func (b atcBalancerBuilder) Name() string {
	return atcBalancerName
}

func (b atcBalancerBuilder) Build(cc balancer.ClientConn, opts balancer.BuildOptions) balancer.Balancer {
	return &atcBalancer{
		cc:       cc,
		opts:     opts,
		rng:      syncrand.Global(),
		eval:     &balancer.ConnectivityStateEvaluator{},
		state:    connectivity.Idle,
		subConns: make(map[Address]subConnInfo, 16),
		scStates: make(map[balancer.SubConn]connectivity.State, 16),
		picker:   nil,
	}
}

var _ balancer.Builder = atcBalancerBuilder{}

// }}}

// type atcBalancer {{{

// TODO: implement sharding
//
// Raises the question of how to compute atcBalancer.state, as
// ConnectivityStateEvaluator doesn't understand sharding.  Could use CSE for
// each shard, but would need to aggregate the states for all shards into a
// master state.

type atcBalancer struct {
	cc       balancer.ClientConn
	opts     balancer.BuildOptions
	rng      *rand.Rand
	eval     *balancer.ConnectivityStateEvaluator
	state    connectivity.State
	subConns map[Address]subConnInfo
	scStates map[balancer.SubConn]connectivity.State
	picker   balancer.Picker
	errs     multierror.Error
}

func (bal *atcBalancer) UpdateClientConnState(ccs balancer.ClientConnState) error {
	Logger().Trace().Interface("ClientConnState", ccs).Msg("UpdateClientConnState")

	seen := make(map[Address]struct{}, len(ccs.ResolverState.Addresses))
	for _, addr := range ccs.ResolverState.Addresses {
		addrList := []Address{addr}
		addrKey := noAttrs(addr)
		seen[addrKey] = struct{}{}
		if sci, ok := bal.subConns[addrKey]; ok {
			sci.attrs = addr.Attributes
			bal.subConns[addrKey] = sci
			bal.cc.UpdateAddresses(sci.sc, addrList)
		} else {
			sc, err := bal.cc.NewSubConn(addrList, balancer.NewSubConnOptions{
				HealthCheckEnabled: true,
			})
			if err != nil {
				Logger().Error().Err(err).Msg("NewSubConn")
				continue
			}
			bal.subConns[addrKey] = subConnInfo{sc: sc, attrs: addr.Attributes}
			bal.scStates[sc] = connectivity.Idle
			sc.Connect()
		}
	}

	for addrKey, sci := range bal.subConns {
		if _, found := seen[addrKey]; !found {
			bal.cc.RemoveSubConn(sci.sc)
			delete(bal.subConns, addrKey)
		}
	}

	if len(ccs.ResolverState.Addresses) == 0 {
		bal.ResolverError(errors.New("produced zero addresses"))
		return balancer.ErrBadResolverState
	}
	return nil
}

func (bal *atcBalancer) ResolverError(err error) {
	Logger().Error().Err(err).Msg("ResolverError")

	bal.errs.Errors = append(bal.errs.Errors, err)

	if len(bal.subConns) == 0 {
		bal.state = connectivity.TransientFailure
	}

	if bal.state == connectivity.TransientFailure {
		bal.picker = bal.regeneratePicker()
		bal.cc.UpdateState(balancer.State{
			ConnectivityState: bal.state,
			Picker:            bal.picker,
		})
	}
}

func (bal *atcBalancer) UpdateSubConnState(sc balancer.SubConn, scs balancer.SubConnState) {
	Logger().Trace().Str("SubConn", fmt.Sprintf("%p", sc)).Interface("SubConnState", scs).Msg("UpdateSubConnState")

	newState := scs.ConnectivityState
	oldState, ok := bal.scStates[sc]
	if !ok {
		return
	}

	if oldState == connectivity.TransientFailure && newState == connectivity.Connecting {
		return
	}

	bal.scStates[sc] = newState

	switch newState {
	case connectivity.Idle:
		sc.Connect()

	case connectivity.Shutdown:
		delete(bal.scStates, sc)

	case connectivity.TransientFailure:
		bal.errs.Errors = append(bal.errs.Errors, scs.ConnectionError)
	}

	bal.state = bal.eval.RecordTransition(oldState, newState)

	if (newState == connectivity.Ready) != (oldState == connectivity.Ready) || bal.state == connectivity.TransientFailure {
		bal.picker = bal.regeneratePicker()
	}

	bal.cc.UpdateState(balancer.State{
		ConnectivityState: bal.state,
		Picker:            bal.picker,
	})
}

func (bal *atcBalancer) Close() {
	// pass
}

func (bal *atcBalancer) regeneratePicker() balancer.Picker {
	if bal.state == connectivity.TransientFailure {
		return errPicker{err: bal.errs.ErrorOrNil()}
	}

	scList := make([]balancer.SubConn, 0, len(bal.subConns))
	dataList := make([]Resolved, 0, len(bal.subConns))
	for addrKey, sci := range bal.subConns {
		if state, ok := bal.scStates[sci.sc]; ok && state == connectivity.Ready {
			addr := addrKey
			addr.Attributes = sci.attrs
			data, _ := GetResolved(addr)
			scList = append(scList, sci.sc)
			dataList = append(dataList, data)
		}
	}

	if len(scList) == 0 {
		return errPicker{err: balancer.ErrNoSubConnAvailable}
	}

	return &atcPicker{
		scList:   scList,
		dataList: dataList,
		next:     uint(bal.rng.Intn(len(scList))),
	}
}

var _ balancer.Balancer = (*atcBalancer)(nil)

// }}}

// type atcPicker {{{

type atcPicker struct {
	scList   []balancer.SubConn
	dataList []Resolved
	mu       sync.Mutex
	next     uint
}

func (p *atcPicker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	p.mu.Lock()
	index := p.next
	p.next = (p.next + 1) % uint(len(p.scList))
	p.mu.Unlock()

	sc := p.scList[index]
	return balancer.PickResult{SubConn: sc}, nil
}

var _ balancer.Picker = (*atcPicker)(nil)

// }}}

// type errPicker {{{

type errPicker struct {
	err error
}

func (p errPicker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	return balancer.PickResult{}, p.err
}

var _ balancer.Picker = errPicker{}

// }}}

func noAttrs(addr Address) Address {
	addr.Attributes = nil
	return addr
}

type subConnInfo struct {
	sc    balancer.SubConn
	attrs *attributes.Attributes
}
