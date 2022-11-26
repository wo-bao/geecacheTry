package geecache

import (
	"geeCacheTry/lru"
	"sync"
)

type cache struct {
	mu sync.Mutex  // 加锁预防并发问题 与 Cache封装到一起
	lru *lru.Cache
	cacheBytes int64  // 缓存容量的阈值
}

func (c *cache) add(key string, value ByteView) {  // 添加数据,加锁
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lru == nil {  // 如果Cache还是空的
		c.lru = lru.New(c.cacheBytes, nil)   // new一个Cache实例
	}
	c.lru.Add(key, value)   // 调用Cache的add添加元素
}

func (c *cache) get(key string) (value ByteView, ok bool) {  // 读数据也要加锁,因为要更新缓存的存储位置:查询的要放到前面第一个
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lru == nil {  // Cache 空的,直接返回
		return
	}
	if v,ok := c.lru.Get(key); ok {    //  key是否存在
		return v.(ByteView), ok    // 以ByteView返回
	}
	return
}
