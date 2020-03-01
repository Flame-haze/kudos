package channelService

import (
	"github.com/kudoochui/kudos/log"
	"github.com/kudoochui/kudos/rpc"
	"github.com/kudoochui/kudos/service/codecService"
	"github.com/kudoochui/kudos/utils/array"
	"sync"
)

type Channel struct {
	name 		string
	group 		map[int64]*rpc.Session			//uid => session
	nodeMap 	map[string][]int64				//address => [sessionId]
	lock 		sync.RWMutex
}

func NewChannel(name string) *Channel {
	return &Channel{
		name:  name,
		group: map[int64]*rpc.Session{},
		nodeMap: map[string][]int64{},
	}
}

// Add user to channel.
func (c *Channel) Add(s *rpc.Session)  {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.group[s.GetUserId()] = s.Clone()

	a := c.nodeMap[s.NodeAddr]
	if a != nil {
		c.nodeMap[s.NodeAddr] = append(a, s.GetSessionId())
	} else {
		a = make([]int64,0)
		c.nodeMap[s.NodeAddr] = append(a, s.GetSessionId())
	}
}

// Remove user from channel.
func (c *Channel) Leave(uid int64)  {
	c.lock.Lock()
	defer c.lock.Unlock()

	s := c.group[uid]
	if s == nil {
		return
	}
	if a, ok := c.nodeMap[s.NodeAddr]; ok {
		c.nodeMap[s.NodeAddr] = array.PullInt64(a, s.GetSessionId())
	}

	delete(c.group, uid)
}

// Get userId array
func (c *Channel) GetMembers() []int64  {
	c.lock.RLock()
	defer c.lock.RUnlock()

	array := make([]int64, len(c.group))
	for k,_ := range c.group {
		array = append(array, k)
	}
	return array
}

// Push message to all the members in the channel
func (c *Channel) PushMessage(route string, msg interface{}) {
	data, err := codecService.GetCodecService().Marshal(msg)
	if err != nil {
		log.Error("marshal error: %v", err)
	}

	c.lock.RLock()
	nodeMap := make(map[string][]int64, 0)
	for k,v := range c.nodeMap {
		nodeMap[k] = v
	}
	defer c.lock.RUnlock()

	for addr, sids := range nodeMap {
		args := &rpc.ArgsGroup{
			Sids:    sids,
			Route: 	 route,
			Payload:  data,
		}
		reply := &rpc.ReplyGroup{}
		rpc.RpcInvoke(addr, "ChannelRemote", "PushMessageByGroup", args, reply)
	}
}
