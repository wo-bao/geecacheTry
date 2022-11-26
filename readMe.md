// Overall flow char										     requsets					        local
// gee := createGroup() --------> /api Service : 9999 ------------------------------> gee.Get(key) ------> g.mainCache.Get(key)
// 						|											^					|
// 						|											|					|remote
// 						v											|					v
// 				cache Service : 800x								|			g.peers.PickPeer(key)
// 						|create hash ring & init peerGetter			|					|
// 						|registry peers write in g.peer				|					|p.httpGetters[p.hashRing(key)]
// 						v											|					|
//			httpPool.Set(otherAddrs...)								|					v
// 		g.peers = gee.RegisterPeers(httpPool)						|			g.getFromPeer(peerGetter, key)
// 						|											|					|
// 						|											|					|
// 						v											|					v
// 		http.ListenAndServe("localhost:800x", httpPool)<------------+--------------peerGetter.Get(key)
// 						|											|
// 						|requsets									|
// 						v											|
// 					p.ServeHttp(w, r)								|
// 						|											|
// 						|url.parse()								|
// 						|--------------------------------------------

主要结构: 
type group struct {
    name string  缓存库名
    getter Getter  回调函数, 用于本地库无数据, 同行也无数据时, 到源找数据。此处留给用户来指定源的位置及如何查找数据
    mainCache *cache  缓存, 封装了lru.Cache 里面的container/list.List 用来保存value  map[key]*List.Element key是关键字,
value是List中的节点,在查询时,通过map找到value节点在list中位置的时间复杂度是O1,取出value的时间复杂度也就是O1,然后list是双向链表,且具备
头结点和尾节点,这样移动节点,删除节点,添加节点的时间复杂度都是O1. 因为LRU需要频繁地移动节点,插入节点与删除节点
    loader singleFlight.Group{mu sync.Mutex, m map[string]*call} mu保护m, m用于存储需要同行存储的key的阻塞call{wg sync.WaitingGroup}
    peers PeerPicker  同行, 在本地找不到时,需要去同行那儿找,则peerpicker要实现找出那个确定同行的方法(pickpeer), 找出这个同行后, 这个同行
需要实现寻找key/value的方法(get), 所以peers(peerpicker)实现的方法(pickpeer)返回一个同行(peergetter),peergetter实现的方法get返回查找结果
这里实现的peerpicker是 -->
type HTTPPool struct {
    self string 自己的URL
    basePath URL后的前缀
    mu sync.Mutex 注册节点时使用  // 为什么需要还暂时不清楚
    peers *consistentHash.Map  存储节点的地方,这里需要实现找出上述peergetter的名字
    httpGetters map[name]*httpGetter  name就是上述peergetter的名字,通过这个map映射出实现了上述get()方法的peergetter,
是这里实现的 type httpGetter struct {
    baseURL string  就是同行的URL
}
}
}

查询流程: 首先注册一个group,填充name,getter,填充peers,也就是注册HTTPPool,所有的节点(httpGetter)都要注册到里面,里的hash存储节点名
通过服务器暴露的9999端口,通过http.Handle(/api, func())注册了路由,通过url: http://localhost:999/api?key=keyname,进行查询,
在func()里从request中取出要查的keyname,调用group.Get(keyName)到本地mainCache中查值,如果没有,开始从同行获取,为了避免缓存击穿,
使用map[key]*call来保存正在执行从同行取值的key, 每次进来,先对map进行加锁, 然后判断key是否存在, 存在,则获取到*call,c.wg.Wait(),等待
前一个查询返回,否则,就new一个*call,*call.wg++, 在map注册, 然后对map解锁, 然后同行请求,返回值赋予c.value,c.wg.Done(),并返回查询到的值,
由于c.wg.Done()了,之前并发请求等待的也不阻塞了,直接返回c.value,由于是同一个map的同一个key,所以只需要查询一次,返回所有请求。
在向同行请求前,先判断要到哪个同行查询。调用peers.pickpeer(key)寻找同行, 在HTTPPool中的一致性map中寻找, 先hash(key),然后取余找到最近的值
通过值(可能是虚拟节点的值),在httpPool的map中找到对应的真实同行名peer(如果同行就是自己,则返回nil)并返回。通过返回的peer在httpGetters中
映射出peer的httpGetter.调用它的Get方法, 调用http.Get(URI),向同行发送请求, 同行在g.peers(httpPool)实现了ServeHTTP方法中,
调用group.Get()来查值并返回。

key---->hash(key)---->v:=hash(key)%len()---->v=getMinDis(v)---->map[v]addr  一致性hash
防止缓存击穿
type group struct {
    mu sync.Mutex   加锁保护m
    m map[string]*call   m在有同行请求来查询时,可能产生写操作,查询完毕会产生删除写操作
}

type call struct {   //  用来保证高并发下多个针对同一个key的请求只执行一个远程查询,每个key查询值生成一个call,查询完就删除
    wg sync.WaitGroup   // 在某个key第一次查询到来时,将key注册到group.m中,wg.Add(1),在第一个查询未完成之前,其他相同key的查询都会wg.Wait()
    value interface{}   // 在第一次查询到时,将值存入*call.value,其他wait的查询也会获得该值,并返回
    err error
}

func (g *Group) Do(key string, fn func(string)(val, err)) () {
    g.mu.Lock()
    if g.m == nil {
        g.m = make(map[string]*call)
    }
    if c,ok := g.m[key]; ok {
        g.mu.Unlock()
        c.wg.Wait()
        return c.val, c.err
    }
    c := new(call)
    g.m[key] = c
    c.wg.Add(1)
    g.mu.Unlock()
    c.value, c.err = fn(key)
    c.wg.Done()
    g.mu.Lock()
    delete(g.m, key)
    g.mu.Unlock()
    return c.value, c.err
    
}

客户查询(传入查询key)--先检查本地缓存(有则返回)--无则先上锁进行防止缓存击穿--根据key到一致性hash的map中寻找对应节点--然后根据对应节点进行查询--
若无结果--调用回调函数源处查数据--期间所有对同一key的查询都会被阻塞--查询结束,其他阻塞的查询也会获得值,将值返回
