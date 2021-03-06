package main

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

/*
https://github.com/patrickmn/go-cache
go-cache 源码阅读和仿写

NOTE : 使用锁的注意事项：

1.如果对性能要求很高，可能需要放弃使用 defer ，直接在需要 unlock 的地方调用 unlock
2.如果没有使用 defer，临界区的 lock 注意 unlock，防止死锁。unlock 和 return 个数是一致的
3 对于读多写少场景，使用 Rlock/RUnlock 性能更好。注意 rlock/runlock 对应，不对应会出现死锁
*/

// 首先是定一个 cache 的 item 对象
type Item struct {
	Object     interface{} // 支持保存不同类型
	Expiration int64       // 判断是否过期用。保存过期的时间戳
}

// 项目是否超时
func (item Item) Expired() bool {
	if item.Expiration == 0 { // 0 是永远不过期
		return false
	}
	return time.Now().UnixNano() > item.Expiration
}

const (
	NoExpiration      time.Duration = -1 // 永不超时
	DefaultExpiration time.Duration = 0  // 默认0
)

// 定义 Cache 结构
type Cache struct {
	*cache
}

func (c *Cache) Print() {
	fmt.Printf("%+v\n", c.cache.items)
}

type cache struct {
	defaultExpiration time.Duration
	items             map[string]Item           // 底层核心就是一个 map[string]Item
	mu                sync.RWMutex              // 读写锁，cache 一般是读多写少的场景
	onEvicted         func(string, interface{}) // 如果key 超时，需要执行的操作
	janitor           *janitor                  // 看门狗
}

// set k/v，注意超时处理
func (c *cache) Set(k string, x interface{}, d time.Duration) {
	var e int64
	if d == DefaultExpiration { // 0
		d = c.defaultExpiration // 这里如果发现 d 用的是默认的，就用 cache
	}
	if d > 0 { // 有超时，加上超时时间。注意用内置的 Add 方法，不要直接用加法
		e = time.Now().Add(d).UnixNano()
	}
	c.mu.Lock() // write lock
	c.items[k] = Item{
		Object:     x,
		Expiration: e,
	}
	c.mu.Unlock() // 这里没用 defer，go defer 有一定的性能消耗。go-cache 里都是直接用原函数
}

// set without lock
func (c *cache) set(k string, x interface{}, d time.Duration) {
	var e int64
	if d == DefaultExpiration {
		d = c.defaultExpiration
	}
	if d > 0 { // 有超时，加上超时时间。注意用内置的 Add 方法
		e = time.Now().Add(d).UnixNano()
	}
	c.items[k] = Item{
		Object:     x,
		Expiration: e,
	}
}

// 用默认时间设置
func (c *cache) SetDefault(k string, x interface{}) {
	c.Set(k, x, DefaultExpiration)
}

// 只有在没有 key 或者已经超时的情况下增加 key
func (c *cache) Add(k string, x interface{}, d time.Duration) error {
	c.mu.Lock()
	_, found := c.get(k)
	if found {
		c.mu.Unlock()
		return fmt.Errorf("Item %s alread exists", k)
	}
	c.set(k, x, d)
	c.mu.Unlock()
	return nil
}

// get without lock
func (c *cache) get(k string) (interface{}, bool) {
	item, found := c.items[k]
	if !found {
		return nil, false
	}
	// 找到还需要判断下是否超时。这里可以看出来是一个 lazy 操作
	if item.Expiration > 0 {
		if time.Now().UnixNano() > item.Expiration {
			return nil, false // timeout
		}
	}
	return item.Object, true
}

// 如果存在就替换
func (c *cache) Replace(k string, x interface{}, d time.Duration) error {
	c.mu.Lock()
	_, found := c.get(k)
	if !found {
		c.mu.Unlock()
		return fmt.Errorf("Item %s doesnt exist", k)
	}
	c.set(k, x, d)
	c.mu.Unlock()
	return nil
}

// Get 获取 item (rlock/unrlock)
func (c *cache) Get(k string) (interface{}, bool) {
	c.mu.RLock()
	item, found := c.items[k]
	if !found {
		c.mu.Unlock()
		return nil, false
	}
	if item.Expiration > 0 { // 过期操作。
		if time.Now().UnixNano() > item.Expiration {
			c.mu.RUnlock()
			return nil, false
		}
	}
	c.mu.RUnlock()
	return item.Object, true
}

// 获取带有超时时间的 k/v
func (c *cache) GetWithExpiration(k string) (interface{}, time.Time, bool) {
	c.mu.RLock()
	item, found := c.items[k]
	if !found {
		c.mu.Unlock()
		return nil, time.Time{}, false
	}
	if item.Expiration > 0 {
		if time.Now().UnixNano() > item.Expiration {
			c.mu.RUnlock()
			return nil, time.Time{}, false
		}
		c.mu.RUnlock()
		return item.Object, time.Unix(0, item.Expiration), true
	}
	c.mu.RUnlock()
	return item.Object, time.Time{}, true
}

func (c *cache) Delete(k string) {
	c.mu.Lock()
	v, evicted := c.delete(k)
	c.mu.Unlock()
	if evicted {
		c.onEvicted(k, v) // 删除的回调
	}
}

func (c *cache) delete(k string) (interface{}, bool) {
	if c.onEvicted != nil { // func
		if v, found := c.items[k]; found {
			delete(c.items, k)
			return v.Object, true
		}
	}
	delete(c.items, k)
	return nil, false
}

type keyAndValue struct {
	key   string
	value interface{}
}

// 删除所有过期 key
func (c *cache) DeleteExpired() {
	var evictedItems []keyAndValue // 记录需要回调的 k/v
	now := time.Now().UnixNano()
	c.mu.Lock()
	for k, v := range c.items {
		if v.Expiration > 0 && now > v.Expiration {
			ov, evicted := c.delete(k)
			if evicted {
				evictedItems = append(evictedItems, keyAndValue{k, ov})
			}
		}
	}
	c.mu.Unlock()
	for _, v := range evictedItems {
		c.onEvicted(v.key, v.value)
	}
}

// 记录当 k/v 移除之后的回调(可选)
func (c *cache) OnEvicted(f func(string, interface{})) {
	c.mu.Lock()
	c.onEvicted = f // 默认 nil 不会执行
	c.mu.Unlock()
}

// Items 复制所有没有超时的 kv 到一个新的 map
func (c *cache) Items() map[string]Item {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now().UnixNano()
	m := make(map[string]Item, len(c.items))
	for k, v := range c.items {
		if v.Expiration > 0 {
			if now > v.Expiration {
				continue
			}
		}
		m[k] = v
	}
	return m
}

// 返回元素个数，可能包含过期的元素
func (c *cache) ItemCount() int {
	c.mu.RLock()
	n := len(c.items)
	c.mu.RUnlock()
	return n
}

// 删除所有
func (c *cache) Flush() {
	c.mu.Lock()
	c.items = map[string]Item{} // clear map
	c.mu.Unlock()
}

//  janitor 用于一定时间(Interval) 执行指定操作
type janitor struct {
	Interval time.Duration
	stop     chan bool // 用来标识结束
}

func (j *janitor) Run(c *cache) {
	ticker := time.NewTicker(j.Interval)
	for {
		select {
		case <-ticker.C: // 每间隔 j.Interval 执行 DeleteExpired()操作
			c.DeleteExpired()
		case <-j.stop:
			ticker.Stop()
			return
		}
	}
}

func stopJanitor(c *Cache) {
	fmt.Println("stopJanitor")
	c.janitor.stop <- true
}

// ci : cleanup interval
func runJanitor(c *cache, ci time.Duration) {
	j := &janitor{
		Interval: ci,
		stop:     make(chan bool),
	}
	c.janitor = j
	go j.Run(c) // NOTE: 定时任务放到一个单独的协程跑
}

func newCache(de time.Duration, m map[string]Item) *cache {
	if de == 0 {
		de = -1
	}
	c := &cache{
		defaultExpiration: de,
		items:             m,
	}
	return c
}

func newCacheWithJanitor(de time.Duration, ci time.Duration, m map[string]Item) *Cache {
	c := newCache(de, m)
	C := &Cache{c}
	if ci > 0 { //  如果需要定期清理
		runJanitor(c, ci) // 每间隔 ci 秒清理过期数据
		// https://zhuanlan.zhihu.com/p/76504936
		runtime.SetFinalizer(C, stopJanitor) // 终止 后台 goroutine，最终被 gc 回收
	}
	return C
}

// defaultExpiration 默认key超时时间，超时之后获取的值是 nil；
// cleanupInterval 定期清理时间。每隔 cleanupInterval 清理*过期* key
func New(defaultExpiration, cleanupInterval time.Duration) *Cache {
	items := make(map[string]Item)
	return newCacheWithJanitor(defaultExpiration, cleanupInterval, items)
}

func main() {
	c := New(time.Minute, time.Minute*2)
	c.Set("k", 123, time.Minute)
	val, ok := c.Get("k")
	c.Print()
	if ok {
		fmt.Println(val.(int))
	}
	time.Sleep(5 * time.Second)
}
