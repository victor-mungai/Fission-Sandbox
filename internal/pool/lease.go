package pool

import "sync"

type Lease struct {
	pool *Pool
	once sync.Once
}

func (l *Lease) Release() {
	if l == nil || l.pool == nil {
		return
	}
	l.once.Do(l.pool.release)
}
