package lru

import "container/list"

type Cache struct {
	maxBytes int64  // 允许的最大内存容量
	usedBytes int64  // 已经占用的内存值
	ll *list.List  // 双向链表,用于数据的保存以及更新数据顺序  // 是带头节点的双向环形链表
	cache map[string]*list.Element  // map用于快速查询数据的存在状态
	OnEvicted func(key string, value Value)
}

type entry struct {
	key string
	value Value
}

type Value interface {
	Len() int
}

func New(maxBytes int64, onEvicted func(string,Value)) *Cache {
	return &Cache{
		maxBytes: maxBytes,
		ll: list.New(),
		cache: make(map[string]*list.Element),
		OnEvicted: onEvicted,
	}
}

func (c *Cache) Get(key string) (value Value, ok bool) {
	if ele,ok := c.cache[key]; ok {   // 查询map中是否有key的节点, ele为ll的节点
		c.ll.MoveToFront(ele)    // 因为被访问了, 移动到队首
		kv := ele.Value.(*entry)    // 取出ll节点的value  entry类型,
		return kv.value, true     //  返回key对应的value
	}
	return 
}

func (c *Cache) RemoveOldest() {
	ele := c.ll.Back()  // 取出队尾节点
	if ele != nil {
		c.ll.Remove(ele)   // 移除该节点
		kv := ele.Value.(*entry)   // 取出该节点的值
		delete(c.cache, kv.key)   // map中删除key
		c.usedBytes -= int64(len(kv.key)) + int64(kv.value.Len())  // 更新因删除节点导致的已使用内存
		if c.OnEvicted != nil {   // 是否执行回调函数
			c.OnEvicted(kv.key, kv.value)
		}
	}
}

func (c *Cache) Add(key string, value Value) {
	if ele,ok := c.cache[key]; ok {   // 如果添加元素已经存在, 则更新值
		c.ll.MoveToFront(ele)    // 把ll节点移动到队首
		kv := ele.Value.(*entry)    // 取出这个存在节点的值
		c.usedBytes += int64(value.Len()) - int64(kv.value.Len())   // 更新由于value值的变化导致的已使用内存的变化
		kv.value = value   //  最后把key对应的value更新
	} else {   //  不存在,则添加
		ele := c.ll.PushFront(&entry{key,value})    // 创建节点,并放置到队首
		c.cache[key] = ele    //  在map中添加该节点的指针
		c.usedBytes += int64(len(key)) + int64(value.Len())  //  更新节点插入导致的已使用内存变化
	}
	for c.maxBytes != 0 && c.maxBytes < c.usedBytes {  // 若导致内存溢出预定值
		c.RemoveOldest()   // 移除多余节点
	}
}

func (c *Cache) Len() int {
	return c.ll.Len()  // 返回链表总长
}