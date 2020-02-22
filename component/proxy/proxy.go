package proxy

import (
	"context"
	"github.com/kudoochui/rpcx/client"
	"github.com/kudoochui/kudos/log"
	"github.com/kudoochui/kudos/rpc"
)

type Proxy struct {
	opts 			*Options

	pool 			*client.OneClientPool
	ChanCall 		chan *rpc.Call
	ChanRet 		chan *client.Call
	responder 		rpc.RpcResponder
}

func NewProxy(opts ...Option) *Proxy {
	options := newOptions(opts...)

	return &Proxy{
		opts:      options,
	}
}

func (r *Proxy) OnInit() {
	r.ChanCall = make(chan *rpc.Call, r.opts.ChanCallSize)
	r.ChanRet = make(chan *client.Call, r.opts.ChanRetSize)
}

func (r *Proxy) OnDestroy() {

}

func (r *Proxy) Run(closeSig chan bool) {
	var d client.ServiceDiscovery
	switch r.opts.RegistryType {
	case "consul":
		d = client.NewConsulDiscovery(r.opts.BasePath, "", []string{r.opts.RegistryAddr}, nil)
	case "etcd":
		d = client.NewEtcdDiscovery(r.opts.BasePath, "", []string{r.opts.RegistryAddr}, nil)
	case "etcdv3":
		d = client.NewEtcdV3Discovery(r.opts.BasePath, "", []string{r.opts.RegistryAddr}, nil)
	case "zookeeper":
		d = client.NewZookeeperDiscovery(r.opts.BasePath, "", []string{r.opts.RegistryAddr}, nil)
	}

	var s client.SelectMode
	switch r.opts.SelectMode {
	case "RoundRobin":
		s = client.RoundRobin
	case "WeightedRoundRobin":
		s = client.WeightedRoundRobin
	case "WeightedICMP":
		s = client.WeightedICMP
	case "ConsistentHash":
		s = client.ConsistentHash
	case "Closest":
		s = client.Closest
	default:
		s = client.RandomSelect
	}

	r.pool = client.NewOneClientPool(r.opts.RpcPoolSize, client.Failtry, s, d, client.DefaultOption)

	for {
		select {
		case <-closeSig:
			goto onEnd
		case ci := <-r.ChanCall:
			r.exec(ci)
		case ri := <-r.ChanRet:
			if ri.Error != nil {
				log.Error("failed to call: %v", ri.Error)
			} else {
				args := ri.Args.(*rpc.Args)
				r.responder.Cb(&args.Session, args.MsgId, ri.Reply)
			}
		}
	}
onEnd:
	log.Info("proxy component closed")
	r.pool.Close()
}

func (r *Proxy) SetRpcResponder(resp rpc.RpcResponder){
	r.responder = resp
}

func (r *Proxy) RpcCall(servicePath string, serviceMethod string, args interface{}, reply interface{}) error {
	xclient := r.pool.Get()
	err := xclient.Call(context.TODO(), servicePath, serviceMethod, args, reply)
	return err
}

func (r *Proxy) Go(call *rpc.Call) {
	r.ChanCall <- call
	//select {
	//case r.ChanCall <- call:
	//	// ok
	//default:
	//	log.Debug("rpc: discarding Call due to insufficient Call chan capacity")
	//}
}

func (r *Proxy) exec(call *rpc.Call) {
	args := &rpc.Args{
		Session: *call.Session,
		MsgId: call.MsgId,
		MsgReq:  call.MsgReq,
	}

	xclient := r.pool.Get()
	if _,err := xclient.Go(context.TODO(), call.ServicePath, call.ServiceName, args, call.MsgResp, r.ChanRet); err != nil {
		log.Error("network call error: %v", err)
	}
}