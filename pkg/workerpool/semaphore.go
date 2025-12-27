package workerpool

type Semaphore chan struct{}

func NewSemaphore(maxConcurrent int) Semaphore {
	return make(chan struct{}, maxConcurrent)
}

func (s Semaphore) Acquire() {
	s <- struct{}{}
}

func (s Semaphore) Release() {
	<-s
}
