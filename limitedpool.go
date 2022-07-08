package limitedpool

import (
	"context"
	"sync"
)

type Creator func () interface{}

type LimitedPool struct {
	mx sync.RWMutex
	free []interface{}
	used int
	queue []*ctxChan
	creator Creator
}

func New(limit int, creator Creator) *LimitedPool {
	return &LimitedPool{
		free: make([]interface{}, 0, limit),
		used: 0,
		queue: make([]*ctxChan, 0),
		creator: creator,
	}
}

func (this *LimitedPool) Get(ctx context.Context) interface{} {
	this.mx.Lock()
	// take one of free items:
	if len(this.free) > 0 {
		res := this.free[0]
		this.free = this.free[1:]
		this.used++
		this.mx.Unlock()
		return res
	}
	// take fresh item:
	if this.used < cap(this.free) {
		this.used++
		this.mx.Unlock()
		res := this.creator()
		return res
	}
	// wait for freed item:
	wait := make(chan interface{})
	cc := &ctxChan{
		done: ctx.Done(),
		wait: wait,
	}
	this.queue = append(this.queue, cc)
	this.mx.Unlock()
	select {
	case res := <-wait:
		return res
	case <-ctx.Done():
		return nil
	}
}

func (this *LimitedPool) Put(item interface{}) {
	this.mx.Lock()
	defer this.mx.Unlock()
	// try to send to the queue:
	for i, cc := range this.queue {
		select {
		case <-cc.done:
			close(cc.wait)
		case cc.wait <- item:
			close(cc.wait)
			this.queue = this.queue[i + 1:]
			return
		}
	}
	this.queue = this.queue[:0]
	// store freed item:
	this.free = append(this.free, item)
	this.used--
}

func (this *LimitedPool) Count() (used int, free int) {
	this.mx.RLock()
	used = this.used
	free = cap(this.free) - used
	this.mx.RUnlock()
	return used, free
}

type ctxChan struct {
	done <-chan struct{}
	wait chan<- interface{}
}
