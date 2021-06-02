package main

import (
	"math"
	"time"

	"github.com/chronos-tachyon/roxy/internal/picker"
	"github.com/chronos-tachyon/roxy/lib/costhistory"
	"github.com/chronos-tachyon/roxy/lib/roxyutil"
	"github.com/chronos-tachyon/roxy/proto/roxy_v0"
)

type ClientData struct {
	ShardData   *ShardData
	UniqueID    string
	Location    Location
	DeclaredCPS float64
	MeasuredCPS float64
	AssignedCPS float64
	IsAlive     bool
	IsServing   bool
	FlushCh     chan struct{}
	GoAwayCh    chan *roxy_v0.GoAway
	ExpireTime  time.Time
	CostHistory costhistory.CostHistory
	Assignments map[string]float64
	EventQueue  []*roxy_v0.Event
}

func (clientData *ClientData) LockedNeededCPS() float64 {
	return math.Max(1.0, math.Max(clientData.DeclaredCPS, clientData.MeasuredCPS))
}

func (clientData *ClientData) LockedSendGoAway(goAway *roxy_v0.GoAway) {
	if clientData.IsAlive {
		select {
		case clientData.GoAwayCh <- goAway:
		default:
		}
	}
}

func (clientData *ClientData) LockedUpdate(isAlive bool, isServing bool) {
	shardData := clientData.ShardData
	histData := clientData.CostHistory.Data()
	now := histData.Now

	isAliveChanged := (clientData.IsAlive != isAlive)
	clientData.IsAlive = isAlive
	if isAliveChanged && !isAlive {
		close(clientData.GoAwayCh)
		close(clientData.FlushCh)
		clientData.ExpireTime = now.Add(ExpireInterval)
	}
	if isAliveChanged && isAlive {
		clientData.ExpireTime = time.Time{}
		clientData.FlushCh = make(chan struct{}, 1)
		clientData.GoAwayCh = make(chan *roxy_v0.GoAway, 1)
	}

	if clientData.IsServing {
		shardData.DeclaredDemandCPS -= clientData.DeclaredCPS
		shardData.MeasuredDemandCPS -= clientData.MeasuredCPS
	}
	clientData.MeasuredCPS = histData.PerSecond
	clientData.IsServing = isServing
	if clientData.IsServing {
		shardData.DeclaredDemandCPS += clientData.DeclaredCPS
		shardData.MeasuredDemandCPS += clientData.MeasuredCPS
	}
}

func (clientData *ClientData) LockedDelete() {
	shardData := clientData.ShardData

	if clientData.IsServing {
		shardData.DeclaredDemandCPS -= clientData.DeclaredCPS
		shardData.MeasuredDemandCPS -= clientData.MeasuredCPS
		clientData.IsServing = false
	}

	for serverID, assignedCPS := range clientData.Assignments {
		serverData := shardData.Servers[serverID]
		serverData.AssignedCPS -= assignedCPS
		clientData.AssignedCPS -= assignedCPS
		shardData.AssignedCPS -= assignedCPS
		delete(serverData.Assignments, clientData.UniqueID)
		delete(clientData.Assignments, serverID)
	}
}

func (clientData *ClientData) LockedInsertServer(serverData *ServerData, newCPS float64, insertEvent bool) {
	oldCPS, found := clientData.Assignments[serverData.UniqueID]
	roxyutil.Assertf(!found, "existing association between client %q and server %q (%f CPS)", clientData.UniqueID, serverData.UniqueID, oldCPS)

	shardData := clientData.ShardData
	shardData.AssignedCPS += newCPS
	serverData.AssignedCPS += newCPS
	serverData.Assignments[clientData.UniqueID] = newCPS
	clientData.AssignedCPS += newCPS
	clientData.Assignments[serverData.UniqueID] = newCPS

	if insertEvent && clientData.IsAlive {
		clientData.EventQueue = append(clientData.EventQueue, &roxy_v0.Event{
			EventType:             roxy_v0.Event_INSERT_IP,
			UniqueId:              serverData.UniqueID,
			Location:              string(serverData.Location),
			ServerName:            serverData.ServerName,
			Ip:                    []byte(serverData.Addr.IP),
			Zone:                  serverData.Addr.Zone,
			Port:                  uint32(serverData.Addr.Port),
			AssignedCostPerSecond: newCPS,
		})
		sendFlush(clientData.FlushCh)
	}
}

func (clientData *ClientData) LockedUpdateServer(serverData *ServerData, newCPS float64, insertEvent bool) {
	oldCPS, found := clientData.Assignments[serverData.UniqueID]
	roxyutil.Assertf(found, "no existing association between client %q and server %q", clientData.UniqueID, serverData.UniqueID)

	deltaCPS := (newCPS - oldCPS)

	shardData := clientData.ShardData
	shardData.AssignedCPS += deltaCPS
	serverData.AssignedCPS += deltaCPS
	serverData.Assignments[clientData.UniqueID] = newCPS
	clientData.AssignedCPS += deltaCPS
	clientData.Assignments[serverData.UniqueID] = newCPS

	if insertEvent && clientData.IsAlive {
		clientData.EventQueue = append(clientData.EventQueue, &roxy_v0.Event{
			EventType:             roxy_v0.Event_UPDATE_WEIGHT,
			UniqueId:              serverData.UniqueID,
			AssignedCostPerSecond: newCPS,
		})
		sendFlush(clientData.FlushCh)
	}
}

func (clientData *ClientData) LockedDeleteServer(serverData *ServerData, insertEvent bool) {
	oldCPS, found := clientData.Assignments[serverData.UniqueID]
	roxyutil.Assertf(found, "no existing association between client %q and server %q", clientData.UniqueID, serverData.UniqueID)

	shardData := clientData.ShardData
	shardData.AssignedCPS -= oldCPS
	serverData.AssignedCPS -= oldCPS
	delete(serverData.Assignments, clientData.UniqueID)
	clientData.AssignedCPS -= oldCPS
	delete(clientData.Assignments, serverData.UniqueID)

	if insertEvent && clientData.IsAlive {
		clientData.EventQueue = append(clientData.EventQueue, &roxy_v0.Event{
			EventType: roxy_v0.Event_DELETE_IP,
			UniqueId:  serverData.UniqueID,
		})
		sendFlush(clientData.FlushCh)
	}
}

func (clientData *ClientData) LockedOnConnect() {
	if clientData.lockedIsOK() {
		shardData := clientData.ShardData
		for serverID, assignedCPS := range clientData.Assignments {
			serverData := shardData.Servers[serverID]
			clientData.EventQueue = append(clientData.EventQueue, &roxy_v0.Event{
				EventType:             roxy_v0.Event_INSERT_IP,
				UniqueId:              serverID,
				Location:              string(serverData.Location),
				ServerName:            serverData.ServerName,
				Ip:                    []byte(serverData.Addr.IP),
				Zone:                  serverData.Addr.Zone,
				Port:                  uint32(serverData.Addr.Port),
				AssignedCostPerSecond: assignedCPS,
			})
		}
		return
	}
	clientData.LockedAssignServers()
}

func (clientData *ClientData) LockedAssignServers() {
	shardData := clientData.ShardData
	maxServers := uint(len(shardData.Servers))
	minServers := maxServers
	if minServers > 3 {
		minServers = 3
	}

	neededCPS := clientData.LockedNeededCPS() - clientData.AssignedCPS
	availableCPS := float64(0.0)

	p1, serverIndexByID1 := clientData.lockedInsertServerPicker()
	for serverID := range clientData.Assignments {
		serverData := shardData.Servers[serverID]
		availableCPS += serverData.LockedAvailableCPS()
		if index, found := serverIndexByID1[serverID]; found {
			p1.Disable(index)
		}
	}

	newServers := make([]*ServerData, 0, maxServers)
	for {
		numServers := uint(len(newServers))
		if numServers >= minServers && (numServers >= maxServers || availableCPS >= neededCPS) {
			break
		}

		index := p1.Pick(nil)
		p1.Disable(index)

		serverData := p1.Get(index).(*ServerData)
		availableCPS += serverData.LockedAvailableCPS()
		newServers = append(newServers, serverData)
	}

	p2, _ := clientData.lockedAssignServerPicker(newServers)
	amounts := p2.Divvy(neededCPS)
	amountsLen := uint(len(amounts))
	newAssignments := make(map[string]float64, amountsLen)
	for index := uint(0); index < amountsLen; index++ {
		suggestedCPS := amounts[index]
		serverData := p2.Get(index).(*ServerData)
		oldCPS := clientData.Assignments[serverData.UniqueID]
		availableCPS := serverData.LockedAvailableCPS()
		newCPS := math.Min(availableCPS, oldCPS+suggestedCPS)
		deltaCPS := (newCPS - oldCPS)
		newAssignments[serverData.UniqueID] = newCPS
		neededCPS -= deltaCPS
	}

	numRemaining := uint(len(amounts))
	for numRemaining != 0 && neededCPS > 0.0 {
		index := p2.Pick(nil)
		p2.Disable(index)

		serverData := p2.Get(index).(*ServerData)
		oldCPS := clientData.Assignments[serverData.UniqueID]
		newCPS := newAssignments[serverData.UniqueID]
		deltaCPS := (newCPS - oldCPS)
		availableCPS := serverData.LockedAvailableCPS() - deltaCPS
		additionalCPS := math.Min(neededCPS, availableCPS)
		newAssignments[serverData.UniqueID] += additionalCPS
		neededCPS -= additionalCPS
	}

	for serverID, newCPS := range newAssignments {
		serverData := shardData.Servers[serverID]
		if _, found := clientData.Assignments[serverID]; found {
			clientData.LockedUpdateServer(serverData, newCPS, true)
		} else {
			clientData.LockedInsertServer(serverData, newCPS, true)
		}
	}
}

func (clientData *ClientData) LockedIsExpired() bool {
	t := clientData.ExpireTime
	if t.IsZero() {
		return false
	}
	now := clientData.CostHistory.Data().Now
	return now.Sub(t) >= 0
}

func (clientData *ClientData) LockedPeriodic() {
	shardData := clientData.ShardData

	clientData.CostHistory.Update()
	histData := clientData.CostHistory.Data()

	oldCPS := clientData.MeasuredCPS
	newCPS := histData.PerSecond
	clientData.MeasuredCPS = newCPS
	if clientData.IsServing {
		shardData.MeasuredDemandCPS += (newCPS - oldCPS)
	}
}

func (clientData *ClientData) LockedToProto() *roxy_v0.ClientData {
	key := clientData.ShardData.Key
	return &roxy_v0.ClientData{
		ServiceName:           string(key.ServiceName),
		ShardNumber:           uint32(key.ShardNumber),
		HasShardNumber:        key.HasShardNumber,
		UniqueId:              clientData.UniqueID,
		Location:              string(clientData.Location),
		DeclaredCostPerSecond: clientData.DeclaredCPS,
		MeasuredCostPerSecond: clientData.MeasuredCPS,
		IsAlive:               clientData.IsAlive,
		IsServing:             clientData.IsServing,
		History:               costhistory.SamplesToProto(clientData.CostHistory.Snapshot()),
	}
}

func (clientData *ClientData) lockedIsOK() bool {
	shardData := clientData.ShardData
	numServers := uint(len(clientData.Assignments))
	minServers := uint(len(shardData.Servers))
	if minServers > 3 {
		minServers = 3
	}

	assignedCPS := clientData.AssignedCPS
	neededCPS := clientData.LockedNeededCPS()
	return (numServers >= minServers && assignedCPS >= neededCPS)
}

func (clientData *ClientData) lockedInsertServerPicker() (picker.Picker, map[string]uint) {
	shardData := clientData.ShardData

	serverMap := make(map[string]uint, len(shardData.Servers))
	serverList := make([]interface{}, 0, len(shardData.Servers))
	for _, serverData := range shardData.Servers {
		if serverData.IsServing {
			serverMap[serverData.UniqueID] = uint(len(serverList))
			serverList = append(serverList, serverData)
		}
	}

	p := picker.Make(serverList, clientData.makeServerScoreFunc())
	return p, serverMap
}

func (clientData *ClientData) lockedAssignServerPicker(newServers []*ServerData) (picker.Picker, map[string]uint) {
	shardData := clientData.ShardData

	serverMap := make(map[string]uint, len(clientData.Assignments))
	serverList := make([]interface{}, 0, len(clientData.Assignments)+len(newServers))
	for serverID := range clientData.Assignments {
		serverData := shardData.Servers[serverID]
		serverMap[serverID] = uint(len(serverList))
		serverList = append(serverList, serverData)
	}
	for _, serverData := range newServers {
		serverMap[serverData.UniqueID] = uint(len(serverList))
		serverList = append(serverList, serverData)
	}

	p := picker.Make(serverList, clientData.makeServerScoreFunc())
	return p, serverMap
}

func (clientData *ClientData) makeServerScoreFunc() func(interface{}) float64 {
	costMap := clientData.ShardData.CostMap
	return func(v interface{}) float64 {
		serverData := v.(*ServerData)

		utilization := serverData.LockedUtilizationRatio()
		if utilization > 1.0 {
			utilization = 1.0
		}

		latency, _ := costMap.Cost(clientData.Location, serverData.Location)

		//                 utilization  ~~  0.000 .. +0.875 .. +1.000
		//          (8.0 * utilization) ~~  0.000 .. +7.000 .. +8.000
		//    (8.0 - 8.0 * utilization) ~~ +8.000 .. +1.000 ..  0.000
		//  ln(8.0 - 8.0 * utilization) ~~ +2.079 ..  0.000 .. -Inf
		// -ln(8.0 - 8.0 * utilization) ~~ -2.079 ..  0.000 .. +Inf
		//
		// (latency / 32.0) means +1.000 score for each +32.0ms of latency
		//
		// ~~ RATIONALE ~~
		//
		// We want utilization >= 1.0 to have a score of +Inf.  There
		// are two natural functions that I automatically reach for
		// when I want a single vertical asymptote: 1/x and log.
		//
		// 1/x comes with a horizontal asymptote as well, which we
		// don't need here; it probably wouldn't _break_ anything,
		// since we have a lower bound on utilization, but it's not
		// natural.  That makes log the natural choice.
		//
		// However, log's vertical asymptote is not oriented the way we
		// want: it approaches -Inf as its argument approaches zero
		// from the positive side, whereas we want +Inf when the
		// argument approaches one from the negative side.  So we apply
		// two negations: we negate the argument, which mirrors log(x)
		// about the Y axis so that we approach -Inf as x approaches 0
		// from the negative side; and we negate the result, which
		// mirrors log(-x) about the X axis so that we approach +Inf as
		// x approaches 0 from the negative side.
		//
		// The result is -log(-x).
		//
		// From there, it's simple to shift the asymptote line by
		// adding 1.0 to the argument.
		//
		// This produces -log(1-x).
		//
		// At this point, we realize that it would be nice if 0
		// utilization yielded a slightly negative score, with a zero
		// score somewhere in the ballpark of 70-90% utilization.  We
		// pick 7/8 = 0.875 utilization because it can be represented
		// exactly in IEEE floating point.  Playing around with the
		// numbers, and recalling that log(1) = 0, it's easy to
		// discover that scaling log's argument by 8 results in
		// utilization 7/8 returning a zero score.
		//
		// Thus, our final function is -log(8*(1-utilization)).
		//
		return (float64(latency) / 32.0) - math.Log(8.0-8.0*utilization)
	}
}

func sendFlush(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}
