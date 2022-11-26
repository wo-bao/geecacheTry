package geecache

import (
	"fmt"
	"geeCacheTry/consistenthash"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const (
	defaultBasePath = "/_geecache/"  // 本机路径前缀
	defaultReplicas = 50
)

type HTTPPool struct {
	self string   //  自己的URL
	basePath string
	mu sync.Mutex
	peers *consistenthash.Map
	httpGetters map[string]*httpGetter
}

func NewHTTPPool(self string) *HTTPPool {
	return &HTTPPool{
		self: self,
		basePath: defaultBasePath,
	}
}

func (p *HTTPPool) Log(format string, v ...interface{}) {
	log.Printf("[Server %s] %s", p.self, fmt.Sprintf(format, v...))
}

func (p *HTTPPool) ServeHTTP(w http.ResponseWriter, r *http.Request) {  // 实现ServeHTTP方法,就是Handler接口,处理请求
	if !strings.HasPrefix(r.URL.Path, p.basePath) {   // 如果url的前缀不是本机, 那你为什么访问我
		panic("HTTPPool serving unexpected path : " + r.URL.Path)
	}
	fmt.Println(r.URL.Path, p.basePath)
	p.Log("%s %s", r.Method, r.URL.Path)    // 日志: 请求方法和,url
	parts := strings.SplitN(r.URL.Path[len(p.basePath):],"/", 2)  // 把请求分割, 0部分是缓存库group的名, 1部分是key
	if len(parts) != 2 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	groupName := parts[0]
	key := parts[1]

	group := GetGroup(groupName)  // 读缓存group
	if group == nil {
		http.Error(w, "no such group : " + groupName, http.StatusNotFound)
	}
	view, err := group.Get(key)    // 从group中查值
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("content-Type", "application/octet-stream")
	w.Write(view.ByteSlice())  // 因为返回的值是个slice,带地址,所以将查的值浅拷贝一份再发送
}

type httpGetter struct {
	baseURL string
}

func (h *httpGetter) Get(group string, key string) ([]byte, error) {
	u := fmt.Sprintf(  // u是生成的url
		"%v%v/%v",
		h.baseURL,
		url.QueryEscape(group),  // 查询字符串是URL的一部分，其中包含可以传递给Web应用程序的数据。该数据需要进行编码，并且使用进行编码
		url.QueryEscape(key),   // 查询字符串是URL的一部分，其中包含可以传递给Web应用程序的数据。该数据需要进行编码，并且使用进行编码
	)
	res, err := http.Get(u)  // 根据url拨号,查询返回值
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned: %v", res.Status)
	}
	bytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %v", err)
	}
	return bytes, nil  // 一切正常, 返回值
}

var _PeerGetter = (*httpGetter)(nil)


func (p *HTTPPool) Set(peers ...string) {  // 注册节点
	p.mu.Lock()
	defer p.mu.Unlock()
	p.peers = consistenthash.New(defaultReplicas, nil)  // 生成管理hash节点的Map
	p.peers.Add(peers...)  // 把节点group名(url:socket)注册到Map中
	p.httpGetters = make(map[string]*httpGetter, len(peers))
	for _, peer := range peers {
		p.httpGetters[peer] = &httpGetter{baseURL: peer + p.basePath}   // 注册访问路径 URI
	}
}


func (p *HTTPPool) PickPeer(key string) (PeerGetter, bool) {  // 选出同行
	p.mu.Lock()
	defer p.mu.Unlock()
	if peer := p.peers.Get(key); peer != "" && peer != p.self {
		p.Log("Pick peer: %s", peer)
		return p.httpGetters[peer], true   // 返回找出的同行
	}
	return nil, false
}