package lab0

import "sync"

type node[T any] struct {
	value T
	next  *node[T]
}

// Queue is a simple FIFO queue that is unbounded in size.
// Push may be called any number of times and is not
// expected to fail or overwrite existing entries.
type Queue[T any] struct {
	// Add your fields here
	head *node[T]
	tail *node[T]
}

// NewQueue returns a new queue which is empty.
func NewQueue[T any]() *Queue[T] {
	return &Queue[T]{
		head: nil,
		tail: nil,
	}
}

// Push adds an item to the end of the queue.
// dont fail, regardless of value of t
func (q *Queue[T]) Push(t T) {
	newNode := &node[T]{value: t, next: nil}
	if q.tail == nil {
		q.head = newNode
		q.tail = newNode
	} else {
		q.tail.next = newNode
		q.tail = newNode
	}
}

// Pop removes an item from the beginning of the queue
// and returns it unless the queue is empty.
//
// If the queue is empty, returns the zero value for T and false.
//
// If you are unfamiliar with "zero values", consider revisiting
// this section of the Tour of Go: https://go.dev/tour/basics/12
func (q *Queue[T]) Pop() (T, bool) {
	if q.head == nil {
		var zero T
		return zero, false
	}

	v := q.head.value
	q.head = q.head.next

	if q.head == nil {
		q.tail = nil
	}

	return v, true

	// var dflt T
	// return dflt, false
}

// ConcurrentQueue provides the same semantics as Queue but
// is safe to access from many goroutines at once.
//
// You can use your implementation of Queue[T] here.
//
// If you are stuck, consider revisiting this section of
// the Tour of Go: https://go.dev/tour/concurrency/9
type ConcurrentQueue[T any] struct {
	// Add your fields here
	mutex sync.Mutex
	queue Queue[T]
}

func NewConcurrentQueue[T any]() *ConcurrentQueue[T] {
	return &ConcurrentQueue[T]{
		queue: *NewQueue[T](),
	}
}

// Push adds an item to the end of the queue
func (q *ConcurrentQueue[T]) Push(t T) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	q.queue.Push(t)
}

// Pop removes an item from the beginning of the queue.
// Returns a zero value and false if empty.
func (q *ConcurrentQueue[T]) Pop() (T, bool) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	return q.queue.Pop()

	// var dflt T
	// return dflt, false
}
