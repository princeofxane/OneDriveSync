package workerpool

import (
	"log"
	"sync"
)

type Pool struct {
	WorkerCount int
	JobQueue    chan Job
}

type Job struct {
	Task func() error
}

func NewPool(workerCount int, jobQueueSize int) *Pool {
	return &Pool{
		WorkerCount: workerCount,
		JobQueue:    make(chan Job, jobQueueSize),
	}
}

func (p *Pool) Close() {
	close(p.JobQueue)
}

func (p *Pool) AddJob(job Job) {
	p.JobQueue <- job
}

func (p *Pool) Execute() {
	sem := NewSemaphore(p.WorkerCount)

	var wg sync.WaitGroup

	for range 10 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			sem.Acquire()
			defer sem.Release()

			for job := range p.JobQueue {
				if err := job.Task(); err != nil {
					log.Printf("Job error: %v", err)
				}
			}
		}()
	}

	wg.Wait() // Blocks until all workers exit
}
