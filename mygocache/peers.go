package mygocache

// PeerPicker 用于根据 key 选择远程节点
type PeerPicker interface {
	PickPeer(key string) (peer PeerGetter, ok bool)
}

// PeerGetter 用于从远程节点获取数据
type PeerGetter interface {
	Get(group string, key string) ([]byte, error)
}
