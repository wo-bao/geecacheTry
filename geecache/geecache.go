package geecache

import (
	"fmt"
	"geeCacheTry/singleflight"
	"log"
	"sync"
)


type Getter interface {
	Get(key string) ([]byte, error)
}

type GetterFunc func(key string) ([]byte, error)

func (f GetterFunc) Get(key string) ([]byte, error) {
	return f(key)
}

type Group struct {
	name string   // 创建的缓存实例的名字
	getter Getter   // 回调函数,当缓存未命中时,调用它加载数据到缓存
	mainCache cache   // 对应的实际缓存库实例
	peers PeerPicker   // 同行
	loader *singleflight.Group //
}

var (
	mu sync.RWMutex   //
	groups = make(map[string]*Group)   //  存储所有的缓存库实例的地址
)

func NewGroup(name string, cacheBytes int64, getter Getter) *Group {
	if getter == nil {   // 必须给回调函数, 用来缓存不存在时, 到有的地方加载
		panic("nil Getter")
	}
	mu.Lock()
	defer mu.Unlock()
	g := &Group{     // 创建缓存实例
		name: name,
		getter: getter,  // 回调函数
		mainCache: cache{cacheBytes: cacheBytes},  // 真正的缓存实例
		loader: &singleflight.Group{},
	}
	groups[name] = g     // 把缓存实例的地址保存到全局变量
	return g
}

func GetGroup(name string) *Group {  // 从全局变量中取出对应名字的缓存库
	mu.RLock()   // 读锁,不影响内部数据结构
	defer mu.RUnlock()
	g := groups[name]
	return g
}

func (g *Group) RegisterPeers(peers PeerPicker) {
	if g.peers != nil {
		panic("RegisterPeerPicker called more than once")
	}
	g.peers = peers
}

func (g *Group) Get(key string) (ByteView, error) {  // 查询数据的入口
	if key == "" {
		return ByteView{}, fmt.Errorf("key is required")
	}
	if v,ok := g.mainCache.get(key); ok {
		log.Println("[GeeCache] hit")
		return v, nil
	}
	return g.load(key)   // 缓存未命中,从源加载数据
}

func (g *Group) load(key string) (value ByteView, err error) {
	viewi, err := g.loader.Do(key, func() (interface{}, error) {   //  防止缓存击穿
		if g.peers != nil {     //  有同行地址, 到同行处加载
			if peer, ok := g.peers.PickPeer(key); ok {    // 先找到同行
				if value, err = g.getFromPeer(peer, key); err == nil {
					return value, nil
				}
				log.Println("[GeeCache] Failed to get from peer", err)
			}
		}
		return g.getLocally(key)  // 到源处加载
	})

	if err == nil {
		return viewi.(ByteView), nil
	}
	return
}

func (g *Group) getFromPeer(peer PeerGetter, key string) (ByteView, error) {  // 从peer同行取值
	bytes, err := peer.Get(g.name, key)
	if err != nil {
		return ByteView{}, err
	}
	return ByteView{b: bytes}, nil
}


func (g *Group) getLocally(key string) (ByteView, error) {  // 从本地源加载获取key
	bytes, err := g.getter.Get(key)  // 回调函数获取值
	if err != nil {
		return ByteView{}, err
	}
	value := ByteView{b: cloneBytes(bytes)}
	g.populateCache(key, value)  // 把值拷贝到缓存中
	return value, nil
}

func (g *Group) populateCache(key string, value ByteView) {  // 加载的数据添加到缓存
	g.mainCache.add(key, value)
}
