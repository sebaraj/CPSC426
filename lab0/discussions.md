## Written Questions (Short Answers)

These questions are meant to test your understanding of the above resources. It may be helpful to complete
these questions and understand the answers before diving into the coding section of this lab.

Keep answers brief.

1. What is the difference between an unbuffered channel and a buffered channel?

- _A buffered channel is a means of communication with the capacity to store messages, allowing
  non-blocking read/write operations (except in the edge cases of a empty/full buffer). This
  facilitates decoupling of the sender/receiver and provides a backlog. Meanwhile, an unbuffered
  channel does now have the capacity to store messages, forcing synchronization in communication
  (i.e. blocking), thus tightly coupling the sender to the receiver._

2. Which is the default in Go? Unbuffered or buffered?

- _By default, channels in Go are unbuffered/blocking._

3. What does the following code do?

```go
func FunWithChannels() {
    ch := make(chan string)

    ch <- "hello world!"

    message := <-ch
    fmt.Println(message)
}
```

- _The code below creates an unbuffered channel of type string, sends the string, 'hello world' into
  the channel, receives the string from the channel, and then prints the received string._

4. In the function signature of `MergeChannels` in `merge_channels.go`:

```go
// Make sure to read the comment block in merge_channels.go
// T is a generic type in this method signature: https://go.dev/tour/generics/2
func MergeChannels[T any](a <-chan T, b <-chan T, out chan<- T) {
```

What is the difference between `<-chan T`, `chan<- T` and `chan T`?

- _The difference between the three is their directionality, with the `<-` restricting
  sending/receiving. A `<-chan T` is a receive-only channel of the generic T, a `chan<- T` is a
  send-only channel of generic T, and a `chan T` is a bidirectional (send and receive) channel._

5. What happens when you read from a closed channel? What about a `nil` channel?

- _When you attempt to read from a closed channel, the default value of the channel type is
  returned. For example, if the channel is of type `int`, then 0 will be the value 'read'/returned
  from a closed channel. However, if you attempt to read from a `nil` channel, the program will block
  (the goroutine will sleep) on that attempted read._

6. When does the following loop terminate?

```go
func FunReadingChannels(ch chan string) {
    for item := range ch {
        fmt.Println(item)
    }
}
```

- The loop does not terminate until the channel is closed. As long as the channel is open, it will
  read values (if the channel is not empty) or block (if the channel is empty). It is important to
  note that this function does not close the channel, and that another goroutine must close it, and
  after the channel is closed, all remaining elements are read and printed from the buffer until it is
  empty, and then the loop terminates.

7. How can you determine if a `context.Context` is done or canceled?

- _You can use the `Done()` method of `context.Context`. For a cancelable context, suppose we use
  `ctx, cancel := context.WithCancel(context.Background())`. When the context finishes and the done
  channel is closed, `case <-ctx.Done()` progresses and we can immediately check `ctx.Err()`, which
  will return `context canceled` is ctx is canceled._

8. What does the following code (most likely) print in the most recent versions of Go (e.g., Go
   1.23)? Why is that?

```go
for i := 1; i <= 3; i++ {
    go func() {
        time.Sleep(time.Duration(i) * time.Second)
        fmt.Printf("%d\n", i)
    }()
}
fmt.Println("all done!")
```

- _The above code only prints `all done!`, as the newly-spawned goroutine with the smallest sleep
  time of 1 second sleeps for a longer time than it takes for the initial goroutine (func main()) to
  spawn the 3 goroutines and print `all done!`. Since the initial goroutine does not wait for
  the other goroutines to complete, it immediately terminates (and halts the execution of other
  goroutines)._

9. What concurrency utility might you use to "fix" **question 8**?

- _`sync.WaitGroup`_ (see code below)

```go
var wg sync.WaitGroup
for i := 1; i <= 3; i++ {
	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		time.Sleep(time.Duration(i) * time.Second)
		fmt.Printf("%d\n", i)
	}(&wg)
}
wg.Wait()
fmt.Println("all done!")
```

10. What is the difference between a mutex (as in `sync.Mutex`) and a semaphore (as in
    `semaphore.Weighted`)?

- _While both are synchronization primitives, a mutex is a (often) simple locking mechanism
  that allows one thread/goroutine to access a shared resource at that given time. While, a semaphore
  is also used to control access to a shared resource with greater flexibility. A binary semaphore,
  with only two states provides very similar mutual exclusion that a traditional lock does, while a
  counting semaphore enables a limited amount of threads/goroutines (maximum set by the initial
  semaphore value) to simultaneously have access to the resource(s)._

11. What does the following code print?

```go
type Bar struct{}
type Foo struct {
    items  []string
	str    string
	num    int
	barPtr *Bar
	bar    Bar
}

func FunWithStructs() {
	var foo Foo
	fmt.Println(foo.items)
	fmt.Println(len(foo.items))
	fmt.Println(foo.items == nil)
	fmt.Println(foo.str)
	fmt.Println(foo.num)
	fmt.Println(foo.barPtr)
	fmt.Println(foo.bar)
}
```

- `[]`\
  `0`\
  `true`\
  ` `\
  `0`\
  `<nil>`\
  `{}`

- The code demonstrates the default initialization values of many types of variables in go.

12. What does `struct{}` in the type `chan struct{}` mean? Why might you use it?

- `struct{}` is an empty struct type of size 0 (no memory overhead), and `chan struct{}` is a
  channel of empty structs/values. Since `struct{}` has no memory overhead, this is an efficient way
  to implement communication channels exclusively used for signaling events between
  threads/goroutines.

13. **Bonus**: What might be different about **question 8** in different versions of Go (assuming
    after applying your fix in **question 9**)?

- In older versions of go, the goroutine captures the loop variable by reference, not value (in c++,
  capturing by [&] rather than [=]). Therefore, by the time that the newly-spawned goroutines wakeup,
  the initial `main()` goroutine has already incremented `i` at the shared address to 4, and all
  spawned goroutines print `4`. This can be fixed by passing `i` by value to each goroutine:

```go
var wg sync.WaitGroup
for i := 1; i <= 3; i++ {
	wg.Add(1)
	go func(int i, wg *sync.WaitGroup) {
		defer wg.Done()
		time.Sleep(time.Duration(i) * time.Second)
		fmt.Printf("%d\n", i)
	}(i, &wg)
}
wg.Wait()
fmt.Println("all done!")
```
