package concurrency

type Limiter struct {
	name      string
	capacity  int
	semaphore chan struct{}
}

type Snapshot struct {
	Name      string `json:"name"`
	Capacity  int    `json:"capacity"`
	InUse     int    `json:"inUse"`
	Available int    `json:"available"`
}

func NewLimiter(name string, maxConcurrency int) *Limiter {
	if maxConcurrency <= 0 {
		return nil
	}
	return &Limiter{
		name:      name,
		capacity:  maxConcurrency,
		semaphore: make(chan struct{}, maxConcurrency),
	}
}

func (l *Limiter) TryAcquire() bool {
	if l == nil {
		return true
	}

	select {
	case l.semaphore <- struct{}{}:
		return true
	default:
		return false
	}
}

func (l *Limiter) Release() {
	if l == nil {
		return
	}

	select {
	case <-l.semaphore:
	default:
	}
}

func (l *Limiter) InUse() int {
	if l == nil {
		return 0
	}
	return len(l.semaphore)
}

func (l *Limiter) Capacity() int {
	if l == nil {
		return 0
	}
	return l.capacity
}

func (l *Limiter) Name() string {
	if l == nil {
		return ""
	}
	return l.name
}

func (l *Limiter) Snapshot() Snapshot {
	if l == nil {
		return Snapshot{}
	}
	inUse := len(l.semaphore)
	return Snapshot{
		Name:      l.Name(),
		Capacity:  l.Capacity(),
		InUse:     inUse,
		Available: l.Capacity() - inUse,
	}
}
