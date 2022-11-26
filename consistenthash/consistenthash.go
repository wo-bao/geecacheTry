package consistenthash

import (
	"hash/crc32"
	"sort"
	"strconv"
)

type Hash func(data []byte) uint32

type Map struct {
	hash Hash          // hash算法
	replicas int          // 每个真实节点对应的虚拟节点数
	keys []int            // hash 环
	hashMap map[int]string    //  存储hash环上对应的节点名
}

func New(replicas int, fn Hash) *Map {    // 创建节点管理
	m := &Map{
		replicas: replicas,
		hash: fn,          //  fn就是自己设定的hash函数,这是依赖注入的一种方式?
		hashMap: make(map[int]string),
	}
	if m.hash == nil {
		m.hash = crc32.ChecksumIEEE    // 默认的hash算法,也可以是用自己的
	}
	return m
}

func (m *Map) Add(keys ...string) {   // 增加节点
	for _,key := range keys {  // 每个key是一个真实节点
		for i:=0; i<=m.replicas; i++ {      // 每个key有replicas个虚拟节点
			hash := int(m.hash([]byte(strconv.Itoa(i) + key)))     // 计算节点的hash值,i==0时为真实节点的值
			m.keys = append(m.keys, hash)     //  将节点的hash值添加到环上
			m.hashMap[hash] = key   // 存储hash值对应的节点,同一个key下的所有hash值的value都是key
		}
	}
	sort.Ints(m.keys)
}

func (m *Map) Get(key string) string {   // 获取要查询数据的节点名
	if len(m.keys) == 0 {  // 环上无hash值,说明无节点
		return ""
	}
	hash := int(m.hash([]byte(key)))   // 计算查询key的hash值
	idx := sort.Search(len(m.keys), func(i int) bool {    // 寻找环上最近的hash值
		return m.keys[i] >= hash   // 找到第一个节点值比hash大的序号,若没有,则返回n,所以下面要取余,到第一个节点取值
	})
	return m.hashMap[m.keys[idx%len(m.keys)]]        // 返回hash值对应的节点值
}