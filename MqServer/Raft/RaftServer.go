package Raft

import (
	mqLog "MqServer/Log"
	"MqServer/Raft/Gob"
	"MqServer/Raft/Net"
	"MqServer/Raft/Persister"
	pb "MqServer/rpc"
	"bytes"
	"context"
	"errors"
	"google.golang.org/grpc"
	"net"
	"sync"
	"sync/atomic"
)

const (
	RfNodeNotFound        = "RfNodeNotFound"
	UnKnownTopicPartition = "UnKnownTopicPartition"
	TopicAlreadyExist     = "TopicAlreadyExist"
)

var (
	raftListenAddr       = ""
	isRaftAddrSet  int32 = 0
)

type RaftNode struct {
	rf    *Raft
	T     string
	P     string
	Peers []*Net.ClientEnd
	me    int
}

type RaftServer struct {
	pb.UnimplementedRaftCallServer
	mu           sync.RWMutex
	server       *grpc.Server
	listener     net.Listener
	Addr         string
	metadataRaft *Raft
	rfs          map[string]map[string]*RaftNode
}

func (rs *RaftServer) Serve() error {
	return rs.server.Serve(rs.listener)
}
func (rs *RaftServer) HeartBeat(_ context.Context, arg *pb.HeartBeatRequest) (rpl *pb.HeartBeatResponse, err error) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	var rf *Raft
	if tp, par := arg.GetTopic(), arg.GetPartition(); tp == "" || par == "" {
		//panic("invalid topic argument and partition argument")
		rf = rs.metadataRaft
	} else {
		rfnode, ok := rs.rfs[tp][par]
		if !ok {
			return nil, errors.New(RfNodeNotFound)
		}
		rf = rfnode.rf
	}
	rfNodeArgs := RequestArgs{}
	err = Gob.NewDecoder(bytes.NewBuffer(arg.Arg)).Decode(&rfNodeArgs)
	if err != nil {
		mqLog.FATAL(err.Error())
	}
	rfNodeReply := RequestReply{}
	rf.HeartBeat(&rfNodeArgs, &rfNodeReply)

	bff := bytes.Buffer{}
	err = Gob.NewEncoder(&bff).Encode(rfNodeReply)
	if err != nil {
		mqLog.FATAL(err.Error())
	}
	if rpl == nil {
		rpl = &pb.HeartBeatResponse{}
	}
	rpl.Topic = arg.Topic
	rpl.Partition = arg.Partition
	rpl.Result = bff.Bytes()
	return rpl, nil
}

func (rs *RaftServer) RequestPreVote(_ context.Context, arg *pb.RequestPreVoteRequest) (rpl *pb.RequestPreVoteResponse, err error) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	var rf *Raft

	if tp, par := arg.GetTopic(), arg.GetPartition(); tp == "" || par == "" {
		//panic("invalid topic argument and partition argument")
		rf = rs.metadataRaft
	} else {
		x, ok := rs.rfs[tp]
		if !ok {
			return nil, errors.New(RfNodeNotFound)
		}
		rfnode, ok := x[par]
		if !ok {
			return nil, errors.New(RfNodeNotFound)
		}
		rf = rfnode.rf
	}

	rfNodeArgs := RequestArgs{}
	err = Gob.NewDecoder(bytes.NewBuffer(arg.Arg)).Decode(&rfNodeArgs)
	if err != nil {
		mqLog.FATAL(err.Error())
	}
	rfNodeReply := RequestReply{}
	rf.RequestPreVote(&rfNodeArgs, &rfNodeReply)

	bff := bytes.Buffer{}
	err = Gob.NewEncoder(&bff).Encode(rfNodeReply)
	if err != nil {
		mqLog.FATAL(err.Error())
	}
	if rpl == nil {
		rpl = &pb.RequestPreVoteResponse{}
	}
	rpl.Topic = arg.Topic
	rpl.Partition = arg.Partition
	rpl.Result = bff.Bytes()
	return rpl, nil
}

func (rs *RaftServer) RequestVote(_ context.Context, arg *pb.RequestVoteRequest) (rpl *pb.RequestVoteResponse, err error) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	var rf *Raft
	if tp, par := arg.GetTopic(), arg.GetPartition(); tp == "" || par == "" {
		//panic("invalid topic argument and partition argument")
		rf = rs.metadataRaft
	} else {
		x, ok := rs.rfs[tp]
		if !ok {
			return nil, errors.New(RfNodeNotFound)
		}
		rfnode, ok := x[par]
		if !ok {
			return nil, errors.New(RfNodeNotFound)
		}
		rf = rfnode.rf
	}
	rfNodeArgs := RequestArgs{}
	err = Gob.NewDecoder(bytes.NewBuffer(arg.Arg)).Decode(&rfNodeArgs)
	if err != nil {
		mqLog.FATAL(err.Error())
	}
	rfNodeReply := RequestReply{}
	rf.RequestVote(&rfNodeArgs, &rfNodeReply)

	bff := bytes.Buffer{}
	err = Gob.NewEncoder(&bff).Encode(rfNodeReply)
	if err != nil {
		mqLog.FATAL(err.Error())
	}
	if rpl == nil {
		rpl = &pb.RequestVoteResponse{}
	}
	rpl.Topic = arg.Topic
	rpl.Partition = arg.Partition
	rpl.Result = bff.Bytes()
	return rpl, nil
}

func (rn *RaftNode) LinkPeerRpcServer(addr string) (*Net.ClientEnd, error) {
	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		panic(err.Error())
	}
	res := &Net.ClientEnd{
		RaftCallClient: pb.NewRaftCallClient(conn),
		Rfn:            rn,
		Conn:           conn,
	}
	return res, nil
}

// url包含自己
func (rs *RaftServer) RegisterRfNode(T, P string, NodesUrl []string, ch chan ApplyMsg) error {
	if T == "" || P == "" {
		return errors.New(UnKnownTopicPartition)
	}
	rn := RaftNode{
		T:     T,
		P:     P,
		Peers: make([]*Net.ClientEnd, len(NodesUrl)),
	}
	selfIndex := -1
	for i, n := range NodesUrl {
		if n == raftListenAddr {
			selfIndex = i
			continue
		} else {
			peer, _ := rn.LinkPeerRpcServer(n)
			rn.Peers[i] = peer
		}
	}
	if selfIndex == -1 {
		panic("register node failed")
	}
	rs.mu.Lock()
	_, ok := rs.rfs[T]
	if !ok {
		rs.rfs[T] = make(map[string]*RaftNode)
	}
	rs.rfs[T][P] = &rn
	rs.mu.Unlock()
	rn.rf = Make(rn.Peers, selfIndex, Persister.MakePersister(), ch)
	return nil
}

func SetRaftListenAddr(addr string) bool {
	if ok := atomic.CompareAndSwapInt32(&isRaftAddrSet, 0, 1); ok {
		raftListenAddr = addr
		return true
	}
	return false
}

func MakeRaftServer() (*RaftServer, error) {
	if raftListenAddr == "" {
		panic("RaftListenAddr must be set")
	}
	lis, err := net.Listen("tcp", raftListenAddr)
	if err != nil {
		mqLog.FATAL(err)
	}
	s := grpc.NewServer()
	res := &RaftServer{
		UnimplementedRaftCallServer: pb.UnimplementedRaftCallServer{},
		mu:                          sync.RWMutex{},
		server:                      s,
		listener:                    lis,
		Addr:                        raftListenAddr,
		rfs:                         make(map[string]map[string]*RaftNode),
	}
	pb.RegisterRaftCallServer(s, res)
	return res, nil
}

func (rs *RaftServer) RegisterMetadataRaft(urls []string, ch chan ApplyMsg) error {
	T, P := "", ""
	rn := RaftNode{
		T:     T,
		P:     P,
		Peers: make([]*Net.ClientEnd, len(urls)),
	}
	selfIndex := -1
	for i, n := range urls {
		if n == raftListenAddr {
			selfIndex = i
			continue
		} else {
			peer, _ := rn.LinkPeerRpcServer(n)
			rn.Peers[i] = peer
		}
	}
	if selfIndex == -1 {
		panic("register node failed")
	}
	rs.mu.Lock()
	_, ok := rs.rfs[T]
	if !ok {
		rs.rfs[T] = make(map[string]*RaftNode)
	}
	rs.rfs[T][P] = &rn
	rs.mu.Unlock()
	rn.rf = Make(rn.Peers, selfIndex, Persister.MakePersister(), ch)
	return nil
}

func (rs *RaftServer) Stop() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rfnode := make([]*RaftNode, 0)
	for _, i := range rs.rfs {
		for _, j := range i {
			rfnode = append(rfnode, j)
		}
	}
	for _, node := range rfnode {
		node.rf.Kill()
		for _, r := range node.Peers {
			r.Conn.Close()
		}
	}
	rs.server.Stop()
	rs.metadataRaft = nil
	rs.rfs = make(map[string]map[string]*RaftNode)
}
