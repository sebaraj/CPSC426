## CPSC 426 - Lab 1 Discussion

#### Bryan SebaRaj

#### 09/25/2024

#### A1. What networking/socket API(s) do you think NewClient(...) call corresponds to or is similar to?

`grpc.NewClient()` corresponds to the `socket` and `connect()` system calls in the socket API, as
NewClient both creates an endpoint for communication with a remote computer and establishes a
new connection to a remote server.

#### A2. In what cases do you think the NewClient() call will fail? What status code do you think you should return for these cases?

`NewClient()` can fail for a multitude of reasons. First, it is possible that the client is using an
improper/malformed address, which should return the InvalidArgument code. Second, it is possible
that the server is unreachable (does not exist/network issues), which should return the Unavailable
code, or that the server does not have the necessary/available resources to handle a new client,
which should return the ResourceExhausted code. Other scenarios include a timeout during connection
(code DeadlineExceeded), TLS/SSL handshake failure (which is not possible in our case) with code
PermissionDenied of Unauthenticaed, and improper gRPC client configuration (FailedPrecondition
code). Finally, the server should be fully functional, but fail to support the gRPC protocl, which
should return a code Unimplemented.

#### A3. What networking/system calls do you think will be used under the hood when you call GetUser()? What cases do you think this call / these calls can return an error? You may find the slides from Lecture 3 helpful. Could GetUser return errors in cases where the network calls succeed?

The `GetUser()` function should use the `send()` syscall to send the request and the `recv()`
syscall to receive the response (in addition to the other HTTP overhead) under the hood.
These calls can return an error is the request times out, the connection is dropped (i.e.
interrupted while sending/receiving), or if the client improperly sends a request. Examples of an
imrpoper request include invalid request data (i.e. invalid arguments), a request where the client
does not have permission, or a request when the client is being rate-limited. In addition, the
server can return an error if it is overloaded.

#### ExtraCredit1. How are these errors detected?

The errors are detected by the respective layer in the client that the error occurs/is recognized by
the server. In other words, if you have socket or transport errors, the client's pc will recognize
this in the netowrk layer, while if you have a gRPC error, the client's pc will recognize this in
the application layer.

#### A4. What would happen if you used the same conn for both VideoService and UserService?

Since the VideoService and UserService are two unique services (running on different
addresses/ports), using the same connection for both services would fail to properly direct traffic
to the appropriate service, as a connection can only connect one gRPC client to one gRPC server. Assuming that the connection is correctly established to one service
(suppose it is the UserService), the VideoService would not be able to receive any requests and all
calls to the VideoService from the VideoRecService would fail, as they are unimplemented in the
UserService.

#### A8. Should you send the batched requests concurrently? Why or why not? What are the advantages or disadvantages?

In many scenarios, sending batched requests concurrently is the best option. Concurrent requests can
drastically reduce the time it takes to process a batch of requests (overall latency), as it
facilitates requests to be sent out while waiting for responses from previous requests. As network
io is notorious for often being the limiting factor (network-bound) in a single-threaded execution,
this circumnavigates this bottleneck. This ultimately improves the throughput of the sending service
(VideoRecService) itself. However, sending batches concurrently also comes with disadvantages. Many
concurrent requests can spike resource usage (such as cpu/memory) and overwhelm the network on both
the client and server sides, leading
to congestion, throttling, timeouts, and packetloss. In addition, implementing concurrancy can lead to race conditions and
data integrity violations. However, for our implementation, these are easily mitigated as only data
integrity needs to be maintained, and the order that the various batches are received from
UserService and VideoService is inconsequential (as long as all UserService data is received before
all VideoService data).

#### ExtraCredit2. Assume that handling one batch request is cheap up to the batch size limit. How could you reduce the total number of requests to UserService and VideoService using batching, assuming you have many incoming requests to VideoRecService? Describe how you would implement this at a high-level (bullet points and pseudocode are fine) but you do not need to implement it in your service.

To reasonably minimize the number of requests to each service, we could batch requests from different GetTopVideo calls together, i.e. from the entire VideoRecService to the other services.

##### High-level implementation:

1. Create two buffers to accumulate requests to the user service and video service respectively
2. When the requests are sent to the buffers, have the calling function (GetTopVideo) append to the
   server-wide channel map for the intended target service, to direct a correct future response back to the calling function
3. When the buffer size is reached or a time limit is exceeded (ideally a fraction of the unbatched
   response time), send the batched requests to the respective service
4. When the response is received, use the channel map to direct the response back to the correct calling
   function

#### B2. Performance metrics for `go run cmd/loadgen/loadgen.go --target-qps=10`:

Using the `loadgen` tool with a target QPS of 10 and the default services (A6), the following performance metrics were recorded:

Stats client metrics:

```
now_us  total_requests  total_errors    active_requests user_service_errors     video_service_errors    average_latency_ms      p99_latency_ms  stale_responses
1726368507778931        0       0       0       0       0       0.00    NaN     0
1726368508773862        0       0       0       0       0       0.00    NaN     0
1726368509778337        23      0       0       0       0       117.00  209.00  0
1726368510783957        23      0       0       0       0       117.00  209.00  0
1726368511780421        23      0       0       0       0       117.00  209.00  0
1726368512780556        38      0       0       0       0       123.00  209.00  0
1726368513780363        49      0       1       0       0       112.00  209.00  0
1726368514779468        59      0       1       0       0       106.00  208.92  0
1726368515780911        68      0       1       0       0       107.00  208.83  0
1726368516780093        79      0       1       0       0       108.00  208.72  0
1726368517780511        89      0       2       0       0       107.00  208.63  0
```

Loadgen metrics:

```
total_sent:20   total_responses:19      total_errors:0  failure_rate:0.00%      stale_responses:0       avg_latency_ms:123.32
total_sent:29   total_responses:29      total_errors:0  failure_rate:0.00%      stale_responses:0       avg_latency_ms:107.07
total_sent:40   total_responses:39      total_errors:0  failure_rate:0.00%      stale_responses:0       avg_latency_ms:103.92
total_sent:49   total_responses:48      total_errors:0  failure_rate:0.00%      stale_responses:0       avg_latency_ms:109.04
total_sent:60   total_responses:59      total_errors:0  failure_rate:0.00%      stale_responses:0       avg_latency_ms:108.27
total_sent:69   total_responses:68      total_errors:0  failure_rate:0.00%      stale_responses:0       avg_latency_ms:110.85
total_sent:79   total_responses:79      total_errors:0  failure_rate:0.00%      stale_responses:0       avg_latency_ms:106.89
total_sent:89   total_responses:88      total_errors:0  failure_rate:0.00%      stale_responses:0       avg_latency_ms:108.86
```

#### C1. Why might retrying be a bad option? In what cases should you not retry a request?

Retrying can be a bad option in cases where the request is idempotent. For
example, if a request to a service is to increment a counter, retrying the request could lead to the
counter being incremented multiple times, which is not the intended behavior. In addition, if the
request is not idempotent, retrying could lead to data corruption or other unintended consequences.
In cases where the request is not idempotent, it is generally not a good idea to retry the request,
as it could lead to data corruption or other unintended consequences.

#### C2. Tradeoffs:

If the videos in the cache are past expiration, I would still return them to the client. Defaulting
to an error would fail to provide the client with any data, which is not ideal. However, returning
old videos, while not ideal, is better than returning no videos at all, and I see the expiration
time as an internal state on when to update the videos, not a hard limit on when to
return/invalidate them. There is a concern that the client may receive very stale data, but this is
a tradeoff I am willing to make to ensure that the client always receives some data. In addition,
the client can always try again (since this is not idempotent) to request new data if they believe the data is too stale.

#### C3. Name at least one additional strategy you could use to improve the reliability of VideoRecService (successful, and useful responses) in the event of failures of UserService or VideoService. What tradeoffs would you make and why?

To improve the reliability of VideoRecService, I would implement a circuit breaker pattern. As
demonstrated by stats.go during periods of high stress, the VideoRecService becomes overwhelmed by
backed up requests due to the unavailability of the UserService and VideoService. I would keep a
running track of errors for each service, and if the errors exceed a threshold over a fixed period
of time, the circuit would trip and all incoming requests to VideoRecService would immediately default to the
cached data (or return an error if the cache is not populated/disabled), preventing calls to the UserService and VideoService. This would prevent the other
services to recover, which I would actively test with background requests to check on their health.
IIf the latency of the test background requests reaches an acceptable level, the circuit would be
turned back on and the other services would be available for request processing again.

#### C4. In part A you likely created new connections via grpc.NewClient() to UserService and VideoService on every request when you needed to use them. What might be costly about connection establishment? (hint: for a high-throughput service you would want to avoid repeated connection establishment.) How could you change your implementation to avoid per-request connection establishment? Does it have any tradeoffs (consider topics such as load balancing from the course lectures)?

The major overhead of conenction establishment is the time it takes (through the network) to
establish the TCP/IP connection (specifically the 3-way handshake). This is costly in terms of
overall latency and to avoid this, we can utilize connection pooling, where we assigned a set of
pre-established connections to a VideoRecService server, and all requests to the server are made
through one of the pooled connections. Similar to network load balancing, a simple round-robin
protocl to assign requests to connections has downsides, as it does not account for the overall
load/health of each connection; as such, some connections may simply not return a response before
the request times out. A more sophisticated load balancing algorithm (such as least connections or
weighted round-robin) which also actively checks the health of each connection would be more
effective, but would require more overhead to implement.
