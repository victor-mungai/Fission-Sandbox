package pool

import "context"

type Recycler struct {
	pool *WarmPool
}

func NewRecycler(pool *WarmPool) *Recycler {
	return &Recycler{pool: pool}
}

func (r *Recycler) Recycle(ctx context.Context, lease *WarmLease, err error) {
	if lease == nil {
		return
	}
	if err != nil {
		lease.Evict(ctx, err.Error())
		return
	}
	lease.Release(ctx)
}
