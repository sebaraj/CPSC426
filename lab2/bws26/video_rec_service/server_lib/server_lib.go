package server_lib

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"cs426.yale.edu/lab1/ranker"
	umc "cs426.yale.edu/lab1/user_service/mock_client"
	upb "cs426.yale.edu/lab1/user_service/proto"
	pb "cs426.yale.edu/lab1/video_rec_service/proto"
	vmc "cs426.yale.edu/lab1/video_service/mock_client"
	vpb "cs426.yale.edu/lab1/video_service/proto"

	"github.com/influxdata/tdigest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type VideoRecServiceOptions struct {
	// Server address for the UserService"
	UserServiceAddr string
	// Server address for the VideoService
	VideoServiceAddr string
	// Maximum size of batches sent to UserService and VideoService
	MaxBatchSize int
	// If set, disable fallback to cache
	DisableFallback bool
	// If set, disable all retries
	DisableRetry bool
	//
	ClientPoolSize int
}

type VideoRecServiceServer struct {
	pb.UnimplementedVideoRecServiceServer
	options VideoRecServiceOptions
	// Add any data you want here

	UserServiceMaxBatchSize  *uint64
	VideoServiceMaxBatchSize *uint64

	// Mock
	isMock                 bool
	mockUserServiceClient  *umc.MockUserServiceClient
	mockVideoServiceClient *vmc.MockVideoServiceClient

	// Stats
	TotalRequests      *uint64
	TotalErrors        *uint64
	ActiveRequests     *uint64
	UserServiceErrors  *uint64
	VideoServiceErrors *uint64
	TotalSumLatencyMs  *uint64
	P99LatencyMs       *float32
	StaleResponses     *uint64
	td                 *tdigest.TDigest

	// Trending Video Cache
	trendingLock        sync.RWMutex
	TrendingCache       []*vpb.VideoInfo
	cacheExpirationTime int64

	// Lab 2C
	UserClientConn  []*grpc.ClientConn
	VideoClientConn []*grpc.ClientConn
	UserConnIndex   *uint64
	VideoConnIndex  *uint64
}

func MakeVideoRecServiceServer(options VideoRecServiceOptions) (*VideoRecServiceServer, error) {
	options.ClientPoolSize = max(1, options.ClientPoolSize)
	server := VideoRecServiceServer{
		options: options,
		// Add any data to initialize here
		UserServiceMaxBatchSize:  new(uint64),
		VideoServiceMaxBatchSize: new(uint64),
		isMock:                   false,
		mockUserServiceClient:    nil,
		mockVideoServiceClient:   nil,
		TotalRequests:            new(uint64),
		TotalErrors:              new(uint64),
		ActiveRequests:           new(uint64),
		UserServiceErrors:        new(uint64),
		VideoServiceErrors:       new(uint64),
		TotalSumLatencyMs:        new(uint64),
		P99LatencyMs:             new(float32),
		StaleResponses:           new(uint64),
		td:                       tdigest.NewWithCompression(10000),
		UserClientConn:           make([]*grpc.ClientConn, options.ClientPoolSize),
		VideoClientConn:          make([]*grpc.ClientConn, options.ClientPoolSize),
		UserConnIndex:            new(uint64),
		VideoConnIndex:           new(uint64),
	}
	*server.UserServiceMaxBatchSize = 1
	*server.VideoServiceMaxBatchSize = 1

	// diail
	var err error
	for i := 0; i < options.ClientPoolSize; i++ {
		server.UserClientConn[i], err = server.ConnectClient(0)
		if err != nil {
			return nil, err
		}
	}
	// defer server.userClientConn.Close()
	for j := 0; j < options.ClientPoolSize; j++ {
		server.VideoClientConn[j], err = server.ConnectClient(1)
		if err != nil {
			return nil, err
		}
	}
	// defer server.videoClientConn.Close()

	go server.UpdateUserServiceMaxBatchSize()
	go server.UpdateVideoServiceMaxBatchSize()
	return &server, nil
}

func MakeVideoRecServiceServerWithMocks(
	options VideoRecServiceOptions,
	mockUserServiceClient *umc.MockUserServiceClient,
	mockVideoServiceClient *vmc.MockVideoServiceClient,
) *VideoRecServiceServer {
	// Implement your own logic here
	server := VideoRecServiceServer{
		options:                  options,
		UserServiceMaxBatchSize:  new(uint64),
		VideoServiceMaxBatchSize: new(uint64),
		isMock:                   true,
		mockUserServiceClient:    mockUserServiceClient,
		mockVideoServiceClient:   mockVideoServiceClient,
		TotalRequests:            new(uint64),
		TotalErrors:              new(uint64),
		ActiveRequests:           new(uint64),
		UserServiceErrors:        new(uint64),
		VideoServiceErrors:       new(uint64),
		TotalSumLatencyMs:        new(uint64),
		P99LatencyMs:             new(float32),
		StaleResponses:           new(uint64),
		td:                       tdigest.NewWithCompression(10000),
		UserClientConn:           nil,
		VideoClientConn:          nil,
		UserConnIndex:            new(uint64),
		VideoConnIndex:           new(uint64),
	}
	*server.UserServiceMaxBatchSize = 1
	*server.VideoServiceMaxBatchSize = 1
	go server.UpdateUserServiceMaxBatchSize()
	go server.UpdateVideoServiceMaxBatchSize()
	return &server
}

// NOTE: need to manually close returned connection
// target: 0 for user service, 1 for video service
func (server *VideoRecServiceServer) ConnectClient(target int) (*grpc.ClientConn, error) {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	var err error
	var conn *grpc.ClientConn
	if target == 0 {
		conn, err = grpc.NewClient(server.options.UserServiceAddr, opts...)
	} else if target == 1 {
		conn, err = grpc.NewClient(server.options.VideoServiceAddr, opts...)
	} else {
		return nil, status.Error(codes.InvalidArgument, "Invalid target argument")
	}
	// NOTE: this error handling code takes inspiration from the sample best-practice code from https://github.com/grpc/grpc-go/blob/master/Documentation/anti-patterns.md
	if err != nil {
		if status, ok := status.FromError(err); ok {
			log.Printf("ConnectClient: RPC error: %v", status.Message())
			return nil, err
		} else {
			log.Printf("ConnectClient: Non-RPC error: %v", err)
			return nil, err
		}
	}
	return conn, nil
}

func (server *VideoRecServiceServer) UpdateCache(
	videoClient vpb.VideoServiceClient,
	maxBatchSize int,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	trendingVideosResponse, err := videoClient.GetTrendingVideos(ctx, &vpb.GetTrendingVideosRequest{})
	if err != nil {
		// Retry
		// atomic.AddUint64(server.TotalErrors, 1)
		atomic.AddUint64(server.VideoServiceErrors, 1)
		if !server.options.DisableRetry {
			ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			trendingVideosResponse, err = videoClient.GetTrendingVideos(ctx, &vpb.GetTrendingVideosRequest{})
			if err != nil {
				atomic.AddUint64(server.TotalErrors, 1)
				atomic.AddUint64(server.VideoServiceErrors, 1)
				return status.Error(codes.Unavailable, "fail to fetch video ids for trending videos")
			}
		} else {
			atomic.AddUint64(server.TotalErrors, 1)
			return status.Error(codes.Unavailable, "fail to fetch video ids for trending videos")
		}
	}
	videoIds := trendingVideosResponse.GetVideos()
	expirationTime := int64(trendingVideosResponse.GetExpirationTimeS())

	var videosInfo []*vpb.VideoInfo
	batchLoopVideo := len(videoIds) / maxBatchSize
	errorChanVideo := make(chan error, batchLoopVideo+1)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i <= batchLoopVideo; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i == batchLoopVideo {
				if len(videoIds)%maxBatchSize != 0 {
					videosInfoResponse, err := videoClient.GetVideo(ctx, &vpb.GetVideoRequest{VideoIds: videoIds[i*maxBatchSize:]})
					if err != nil {
						// atomic.AddUint64(server.TotalErrors, 1)
						atomic.AddUint64(server.VideoServiceErrors, 1)
						if !server.options.DisableRetry {
							videosInfoResponse, err = videoClient.GetVideo(ctx, &vpb.GetVideoRequest{VideoIds: videoIds[i*maxBatchSize:]})
							if err != nil {
								atomic.AddUint64(server.TotalErrors, 1)
								atomic.AddUint64(server.VideoServiceErrors, 1)
								errorChanVideo <- status.Errorf(codes.Unavailable, "GetTopVideos: GetVideo failed: %v", err)
								return
							}
						} else {
							atomic.AddUint64(server.TotalErrors, 1)
							errorChanVideo <- status.Errorf(codes.Unavailable, "GetTopVideos: GetVideo failed: %v", err)
							return
						}
					}
					mu.Lock()
					videosInfo = append(videosInfo, videosInfoResponse.GetVideos()...)
					mu.Unlock()
				}
			} else {
				videosInfoResponse, err := videoClient.GetVideo(ctx, &vpb.GetVideoRequest{VideoIds: videoIds[i*maxBatchSize : (i+1)*maxBatchSize]})
				if err != nil {
					// atomic.AddUint64(server.TotalErrors, 1)
					atomic.AddUint64(server.VideoServiceErrors, 1)
					if !server.options.DisableRetry {
						videosInfoResponse, err = videoClient.GetVideo(ctx, &vpb.GetVideoRequest{VideoIds: videoIds[i*maxBatchSize : (i+1)*maxBatchSize]})
						if err != nil {
							atomic.AddUint64(server.TotalErrors, 1)
							atomic.AddUint64(server.VideoServiceErrors, 1)
							errorChanVideo <- status.Errorf(codes.Unavailable, "GetTopVideos: GetVideo failed: %v", err)
							return
						}
					} else {
						atomic.AddUint64(server.TotalErrors, 1)
						errorChanVideo <- status.Errorf(codes.Unavailable, "GetTopVideos: GetVideo failed: %v", err)
						return
					}
				}
				mu.Lock()
				videosInfo = append(videosInfo, videosInfoResponse.GetVideos()...)
				mu.Unlock()
			}
			errorChanVideo <- nil
		}(i)
	}

	wg.Wait()
	close(errorChanVideo)

	for err := range errorChanVideo {
		if err != nil {
			if server.TrendingCache == nil && videosInfo != nil {
				server.TrendingCache = videosInfo
			}
			return err
		}
	}

	if expirationTime > server.cacheExpirationTime {
		server.TrendingCache = videosInfo
		server.cacheExpirationTime = expirationTime
		// println(server.cacheExpirationTime)
	}

	return nil
}

func (server *VideoRecServiceServer) ContinuallyRefreshCache() {
	// if DisableFallback is true or if isCacheRUnning is already true (i.e. cache is already running) stop execution
	if server.options.DisableFallback {
		return
	}
	var maxBatchSize int
	var videoClient vpb.VideoServiceClient
	for {
		maxBatchSize = int(atomic.LoadUint64(server.VideoServiceMaxBatchSize)) // *server.VideoServiceMaxBatchSize)
		var err error
		if videoClient == nil {
			if server.isMock {
				videoClient = server.mockVideoServiceClient
			} else {
				idx := atomic.AddUint64(server.VideoConnIndex, 1) % uint64(server.options.ClientPoolSize)
				videoClient = vpb.NewVideoServiceClient(server.VideoClientConn[idx])
			}
		}

		if videoClient != nil {
			timeUntilExpiration := time.Duration(server.cacheExpirationTime-(time.Now().UnixNano()/int64(1000*time.Millisecond))) * time.Millisecond
			server.trendingLock.Lock()

			// println("timeUntilExpiration", timeUntilExpiration)
			if timeUntilExpiration < 30*time.Millisecond {
				// refresh cache
				err = server.UpdateCache(videoClient, maxBatchSize)
				server.trendingLock.Unlock()
				if err == nil {
					// time.Sleep(time.Duration(server.cacheExpirationTime-time.Now().UnixNano()/int64(time.Millisecond))*time.Millisecond - 100*time.Millisecond)
					// println("cacheExpirationTime", server.cacheExpirationTime)
					// println("currentTime", time.Now().UnixNano()/int64(1000*time.Millisecond))

					timeUntilNextExpiration := time.Duration(server.cacheExpirationTime-(time.Now().UnixNano()/int64(1000*time.Millisecond))) * time.Millisecond
					sleepDuration := (timeUntilNextExpiration - 100) * 10
					// println("sleepduration", sleepDuration)
					if sleepDuration > 0 {
						time.Sleep(sleepDuration)
					} else {
						time.Sleep(-sleepDuration)
					}
				}
			} else {
				server.trendingLock.Unlock()
				// time.Sleep(timeUntilExpiration - 100*time.Millisecond)
				// println("checking is I am here")
				sleepDuration := (timeUntilExpiration - 100) * 10
				// println("sleepduration2", sleepDuration)
				if sleepDuration > 0 {
					time.Sleep(sleepDuration)
				}
			}
		}

		if err != nil {
			time.Sleep(10 * time.Second)
		}
		// println(len(server.TrendingCache))
		// fmt.Printf("%+v\n", server.TrendingCache)

	}
}

func (server *VideoRecServiceServer) GetCachedVideos(requestLimit int32) []*vpb.VideoInfo {
	server.trendingLock.RLock()
	defer server.trendingLock.RUnlock()

	if server.TrendingCache == nil {
		return nil
	}

	vids := server.TrendingCache
	if requestLimit > 0 && requestLimit <= int32(len(vids)) {
		vids = vids[:requestLimit]
	}
	return vids
}

// Non-error message parsing (i.e. only uses error codes) version of UpdateUserServiceMaxBatchSize
// func (server *VideoRecServiceServer) UpdateUserServiceMaxBatchSize() {
// 	var userClient upb.UserServiceClient
// 	if server.isMock {
// 		userClient = server.mockUserServiceClient
// 	} else {
// 		connUser, err := ConnectClient(server, 0)
// 		for {
// 			if err == nil {
// 				break
// 			}
// 			time.Sleep(10 * time.Millisecond)
// 			connUser, err = ConnectClient(server, 0)
// 		}
// 		defer connUser.Close()
// 		userClient = upb.NewUserServiceClient(connUser)
// 	}
// 	visited := make(map[int]bool)
// 	highestBatchSize := server.options.MaxBatchSize
// 	lowestBatchSize := 1
// 	if lowestBatchSize >= highestBatchSize {
// 		return
// 	}
// 	var mid int
// 	for {
// 		if lowestBatchSize >= highestBatchSize {
// 			highestBatchSize--
// 			lowestBatchSize = highestBatchSize
// 			break
// 		}
// 		mid = (highestBatchSize + lowestBatchSize) / 2
// 		if visited[mid] {
// 			break
// 		}
// 		visited[mid] = true
// 		temp := make([]uint64, mid)
//
// 		for i := 0; i < mid; i++ {
// 			temp[i] = 204054
// 		}
// 		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
// 		defer cancel()
// 		_, err := userClient.GetUser(ctx, &upb.GetUserRequest{UserIds: temp})
// 		for {
// 			if err == nil {
// 				lowestBatchSize = mid + 1
// 				break
// 			}
// 			grpcStatus, ok := status.FromError(err)
// 			if ok {
// 				if grpcStatus.Code() == codes.InvalidArgument {
// 					highestBatchSize = mid
// 					break
// 				} else if grpcStatus.Code() == codes.NotFound {
// 					lowestBatchSize = mid + 1
// 					break
// 				}
//
// 				// else Code() is Internal, so sleep to not overload
// 			}
// 			time.Sleep(10 * time.Millisecond)
// 			_, err = userClient.GetUser(ctx, &upb.GetUserRequest{UserIds: temp})
//
// 		}
//
// 	}
// 	mid = (highestBatchSize + lowestBatchSize) / 2
// 	// println("mid: %d\n", mid)
// 	atomic.AddUint64(server.UserServiceMaxBatchSize, uint64(max(0, mid-1)))
// 	// println("UserServiceMaxBatchSize: ", *server.UserServiceMaxBatchSize)
// }

func (server *VideoRecServiceServer) UpdateUserServiceMaxBatchSize() {
	var userClient upb.UserServiceClient
	if server.isMock {
		userClient = server.mockUserServiceClient
	} else {
		idx := atomic.AddUint64(server.UserConnIndex, 1) % uint64(server.options.ClientPoolSize)
		userClient = upb.NewUserServiceClient(server.UserClientConn[idx])
	}
	highestBatchSize := server.options.MaxBatchSize
	if highestBatchSize < 1 {
		return
	}
	temp := make([]uint64, highestBatchSize)
	for i := 0; i < highestBatchSize; i++ {
		temp[i] = 204054
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for {
		_, err := userClient.GetUser(ctx, &upb.GetUserRequest{UserIds: temp})
		if err == nil {
			atomic.AddUint64(server.UserServiceMaxBatchSize, uint64(max(0, highestBatchSize-1)))
			break
		} else {
			// println("err: ", err)
			grpcStatus, ok := status.FromError(err)
			if ok {
				if grpcStatus.Code() == codes.InvalidArgument {
					// println("grpcStatus.Code(): ", grpcStatus.Code())
					// parse
					message := grpcStatus.Message()
					var lowestBatchSize int
					_, parseErr := fmt.Sscanf(message, "UserService: user_ids exceeded the max batch size %d", &lowestBatchSize)
					if parseErr == nil {
						atomic.AddUint64(server.UserServiceMaxBatchSize, uint64(max(0, lowestBatchSize-1)))
					} // else {
					// println("parseErr: ", parseErr)
					// }
					break

				} else if grpcStatus.Code() == codes.NotFound {
					atomic.AddUint64(server.UserServiceMaxBatchSize, uint64(max(0, highestBatchSize-1)))
					break
				}
				// println("grpcStatus.Code(): ", grpcStatus.Code())
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	// println("UserServiceMaxBatchSize: ", atomic.LoadUint64(server.UserServiceMaxBatchSize))
}

func (server *VideoRecServiceServer) UpdateVideoServiceMaxBatchSize() {
	var videoClient vpb.VideoServiceClient
	if server.isMock {
		videoClient = server.mockVideoServiceClient
	} else {
		idx := atomic.AddUint64(server.VideoConnIndex, 1) % uint64(server.options.ClientPoolSize)
		videoClient = vpb.NewVideoServiceClient(server.VideoClientConn[idx])
	}
	highestBatchSizeV := server.options.MaxBatchSize
	if highestBatchSizeV < 1 {
		return
	}
	temp := make([]uint64, highestBatchSizeV)
	for i := 0; i < highestBatchSizeV; i++ {
		temp[i] = 1300 + uint64(i)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for {
		_, err := videoClient.GetVideo(ctx, &vpb.GetVideoRequest{VideoIds: temp})
		if err == nil {
			atomic.AddUint64(server.VideoServiceMaxBatchSize, uint64(max(0, highestBatchSizeV-1)))
			break
		} else {
			// println("err: ", err)
			grpcStatus, ok := status.FromError(err)
			if ok {
				if grpcStatus.Code() == codes.InvalidArgument {
					// println("grpcStatus.Code(): ", grpcStatus.Code())
					// parse
					message := grpcStatus.Message()
					var lowestBatchSizeV int
					_, parseErr := fmt.Sscanf(message, "VideoService: video_ids exceeded the max batch size %d", &lowestBatchSizeV)
					if parseErr == nil {
						atomic.AddUint64(server.VideoServiceMaxBatchSize, uint64(max(0, lowestBatchSizeV-1)))
					} // else {
					// println("parseErr: ", parseErr)
					// }
					break

				} else if grpcStatus.Code() == codes.NotFound {
					atomic.AddUint64(server.VideoServiceMaxBatchSize, uint64(max(0, highestBatchSizeV-1)))
					break
				}
				// println("grpcStatus.Code(): ", grpcStatus.Code())
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	// println("VideoServiceMaxBatchSize: ", atomic.LoadUint64(server.VideoServiceMaxBatchSize))
}

// Non-error message parsing (i.e. only uses error codes) version of UpdateVideoServiceMaxBatchSize
// func (server *VideoRecServiceServer) UpdateVideoServiceMaxBatchSize() {
// 	var videoClient vpb.VideoServiceClient
// 	if server.isMock {
// 		videoClient = server.mockVideoServiceClient
// 	} else {
// 		connVideo, err := ConnectClient(server, 1)
// 		for {
// 			if err == nil {
// 				break
// 			}
// 			time.Sleep(10 * time.Millisecond)
// 			connVideo, err = ConnectClient(server, 1)
// 		}
// 		defer connVideo.Close()
// 		videoClient = vpb.NewVideoServiceClient(connVideo)
// 	}
// 	visitedV := make(map[int]bool)
// 	highestBatchSizeV := server.options.MaxBatchSize
// 	// println(highestBatchSizeV)
// 	lowestBatchSizeV := 1
// 	// println("highestBatchSizeV: %d\n", highestBatchSizeV)
// 	// println("lowestBatchSizeV: %d\n", lowestBatchSizeV)
// 	if lowestBatchSizeV >= highestBatchSizeV {
// 		return
// 	}
// 	var midV int
// 	for {
// 		if lowestBatchSizeV >= highestBatchSizeV {
// 			highestBatchSizeV--
// 			lowestBatchSizeV = highestBatchSizeV
// 			break
// 		}
//
// 		midV = (highestBatchSizeV + lowestBatchSizeV) / 2
// 		// println("midV: %d\n", midV)
// 		if visitedV[midV] {
// 			break
// 		}
// 		visitedV[midV] = true
// 		tempV := make([]uint64, midV)
//
// 		for i := 0; i < midV; i++ {
// 			tempV[i] = uint64(1300 + i)
// 		}
// 		// println("tempV: ")
// 		// println(tempV)
// 		// for _, v := range tempV {
// 		// print(v, " ")
// 		// }
// 		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
// 		defer cancel()
// 		_, err := videoClient.GetVideo(ctx, &vpb.GetVideoRequest{VideoIds: tempV})
// 		for {
// 			if err == nil {
// 				lowestBatchSizeV = midV + 1
// 				break
// 			}
// 			grpcStatus, ok := status.FromError(err)
// 			if ok {
// 				if grpcStatus.Code() == codes.InvalidArgument {
// 					highestBatchSizeV = midV
// 					break
// 				} else if grpcStatus.Code() == codes.NotFound {
// 					lowestBatchSizeV = midV + 1
// 					break
// 				}
// 			}
// 			time.Sleep(10 * time.Millisecond)
// 			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
// 			defer cancel()
// 			_, err = videoClient.GetVideo(ctx, &vpb.GetVideoRequest{VideoIds: tempV})
// 			if ctx.Err() == context.DeadlineExceeded {
// 				time.Sleep(1 * time.Second)
// 			}
// 		}
//
// 	}
// 	midV = (highestBatchSizeV + lowestBatchSizeV) / 2
// 	// println("mid: %d\n", midV)
// 	atomic.AddUint64(server.VideoServiceMaxBatchSize, uint64(max(0, midV-1)))
// 	// println("VideoServiceMaxBatchSize: ", *server.VideoServiceMaxBatchSize)
// }

func rankVideos(
	userCoeff *upb.UserCoefficients,
	videoCoeffs *[]*vpb.VideoInfo,
	limit int32,
) []*vpb.VideoInfo {
	ranker := ranker.BcryptRanker{}
	likedVideosRanked := make([]*vpb.VideoInfo, 0)
	likedVideosRankedMap := make(map[*vpb.VideoInfo]uint64)

	for _, v := range *videoCoeffs {
		videoCoefficient := v.GetVideoCoefficients()
		rank := ranker.Rank(userCoeff, videoCoefficient)

		likedVideosRanked = append(likedVideosRanked, v)
		likedVideosRankedMap[v] = rank
	}
	sort.SliceStable(likedVideosRanked, func(i, j int) bool {
		return likedVideosRankedMap[likedVideosRanked[i]] > likedVideosRankedMap[likedVideosRanked[j]]
	})
	if limit > 0 && limit <= int32(len(likedVideosRanked)) {
		likedVideosRanked = likedVideosRanked[:limit]
	}
	// for i, v := range likedVideosRanked {
	// 	rank := likedVideosRankedMap[v]
	// 	videoID := v.GetVideoId()
	// 	fmt.Printf("Rank %d: %d, Video ID: %d\n", i, rank, videoID)
	// }

	return likedVideosRanked
}

func (server *VideoRecServiceServer) GetTopVideos(
	ctx context.Context,
	req *pb.GetTopVideosRequest,
) (*pb.GetTopVideosResponse, error) {
	//
	atomic.AddUint64(server.TotalRequests, 1)
	atomic.AddUint64(server.ActiveRequests, 1)
	startTime := time.Now()
	defer func() {
		atomic.AddUint64(server.ActiveRequests, ^uint64(0))
		latency := time.Since(startTime).Milliseconds()
		atomic.AddUint64(server.TotalSumLatencyMs, uint64(latency))
		server.td.Add(float64(latency), 1)
	}()
	// if mode, connect to VideoRecServiceServer.
	if req.GetLimit() < 0 {
		return nil, status.Error(codes.InvalidArgument, "GetTopVideos: invalid limit")
	}
	var userClient upb.UserServiceClient
	if server.isMock {
		userClient = server.mockUserServiceClient
	} else {
		idx := atomic.AddUint64(server.UserConnIndex, 1) % uint64(server.options.ClientPoolSize)
		userClient = upb.NewUserServiceClient(server.UserClientConn[idx])
	}
	var mu sync.Mutex
	var wg sync.WaitGroup

	subscriberUserID := req.GetUserId()

	// userID is 0 if user is not found in DB, so check that 0 is not passed as userID
	if subscriberUserID == 0 {
		atomic.AddUint64(server.TotalErrors, 1)
		// atomic.AddUint64(server.UserServiceErrors, 1)
		if !server.options.DisableFallback {
			cachedVideos := server.GetCachedVideos(req.GetLimit())
			if cachedVideos != nil {
				atomic.AddUint64(server.StaleResponses, 1)
				return &pb.GetTopVideosResponse{Videos: cachedVideos, StaleResponse: true}, nil
			}

		}
		return nil, status.Error(
			// error code
			codes.InvalidArgument,
			"GetTopVideos: invalid user ID",
		)
	}
	subscriberUserResponse, err := userClient.GetUser(ctx, &upb.GetUserRequest{UserIds: []uint64{subscriberUserID}})
	if err != nil {
		// atomic.AddUint64(server.TotalErrors, 1)
		atomic.AddUint64(server.UserServiceErrors, 1)
		if !server.options.DisableRetry {
			subscriberUserResponse, err = userClient.GetUser(ctx, &upb.GetUserRequest{UserIds: []uint64{subscriberUserID}})
			if err != nil {
				atomic.AddUint64(server.TotalErrors, 1)
				atomic.AddUint64(server.UserServiceErrors, 1)
				if !server.options.DisableFallback {
					cachedVideos := server.GetCachedVideos(req.GetLimit())
					if cachedVideos != nil {
						atomic.AddUint64(server.StaleResponses, 1)

						return &pb.GetTopVideosResponse{Videos: cachedVideos, StaleResponse: true}, nil
					}
				}
				if s, ok := status.FromError(err); ok {
					return nil, s.Err()
				}
				return nil, status.Error(codes.Internal, "GetTopVideos: GetUser failed")
			}

		} else {
			atomic.AddUint64(server.TotalErrors, 1)
			if !server.options.DisableFallback {
				cachedVideos := server.GetCachedVideos(req.GetLimit())
				if cachedVideos != nil {
					atomic.AddUint64(server.StaleResponses, 1)
					return &pb.GetTopVideosResponse{Videos: cachedVideos, StaleResponse: true}, nil
				}
			}
			if s, ok := status.FromError(err); ok {
				return nil, s.Err()
			}
			return nil, status.Error(codes.Internal, "GetTopVideos: GetUser failed")
		}
	}
	subscriberUserInfo := subscriberUserResponse.GetUsers()[0] // UserInfo
	subscribedTo := subscriberUserInfo.GetSubscribedTo()       // []uint64
	if len(subscribedTo) == 0 {
		atomic.AddUint64(server.TotalErrors, 1)
		if !server.options.DisableFallback {
			cachedVideos := server.GetCachedVideos(req.GetLimit())
			if cachedVideos != nil {
				atomic.AddUint64(server.StaleResponses, 1)
				rankedCachedVideos := rankVideos(subscriberUserInfo.GetUserCoefficients(), &cachedVideos, req.GetLimit())
				return &pb.GetTopVideosResponse{Videos: rankedCachedVideos, StaleResponse: true}, nil
			}

		}
		return nil, status.Error(
			// error code
			codes.FailedPrecondition,
			"GetTopVideos: user is not subscribed to anyone",
		)
	}
	maxUserBatchSize := int(atomic.LoadUint64(server.UserServiceMaxBatchSize)) // int(*server.UserServiceMaxBatchSize)
	batchLoopUser := len(subscribedTo) / maxUserBatchSize                      // maxBatchSize
	var subscribedToUsers []*upb.UserInfo
	errorChanUser := make(chan error, batchLoopUser+1)
	returnChanCache := make(chan []*vpb.VideoInfo, batchLoopUser+1)
	done := make(chan struct{})
	for i := 0; i <= batchLoopUser; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i == batchLoopUser {
				// maxBatchSize
				if len(subscribedTo)%maxUserBatchSize != 0 {
					subscribedToUsersResponse, err := userClient.GetUser(ctx, &upb.GetUserRequest{UserIds: subscribedTo[i*maxUserBatchSize:]})
					if err != nil {
						// errorChanUser <- status.Errorf(codes.Unavailable, "GetTopVideos: GetUser call failed: %v", err)
						// atomic.AddUint64(server.TotalErrors, 1)
						atomic.AddUint64(server.UserServiceErrors, 1)
						if !server.options.DisableRetry {
							subscribedToUsersResponse, err = userClient.GetUser(ctx, &upb.GetUserRequest{UserIds: subscribedTo[i*maxUserBatchSize:]})
							if err != nil {
								// atomic.AddUint64(server.TotalErrors, 1)
								atomic.AddUint64(server.UserServiceErrors, 1)
								if !server.options.DisableFallback {
									cachedVideos := server.GetCachedVideos(req.GetLimit())
									if cachedVideos != nil {
										returnChanCache <- cachedVideos
										// return
									}
									return
								}
								errorChanUser <- status.Errorf(codes.Unavailable, "GetTopVideos: GetUser call failed: %v", err)
								return
							}
						} else {
							// atomic.AddUint64(server.TotalErrors, 1)
							if !server.options.DisableFallback {
								cachedVideos := server.GetCachedVideos(req.GetLimit())
								if cachedVideos != nil {
									returnChanCache <- cachedVideos
									// return
								}
								return
							}
							errorChanUser <- status.Errorf(codes.Unavailable, "GetTopVideos: GetUser call failed: %v", err)
							return
						}

					}
					// Append the result to the subscribedToUsers slice
					mu.Lock()
					subscribedToUsers = append(subscribedToUsers, subscribedToUsersResponse.GetUsers()...)
					mu.Unlock()
				}
			} else {
				subscribedToUsersResponse, err := userClient.GetUser(ctx, &upb.GetUserRequest{UserIds: subscribedTo[i*maxUserBatchSize : (i+1)*maxUserBatchSize]})
				if err != nil {
					// atomic.AddUint64(server.TotalErrors, 1)
					atomic.AddUint64(server.UserServiceErrors, 1)
					if !server.options.DisableRetry {
						subscribedToUsersResponse, err = userClient.GetUser(ctx, &upb.GetUserRequest{UserIds: subscribedTo[i*maxUserBatchSize : (i+1)*maxUserBatchSize]})
						if err != nil {
							// atomic.AddUint64(server.TotalErrors, 1)
							atomic.AddUint64(server.UserServiceErrors, 1)
							if !server.options.DisableFallback {
								cachedVideos := server.GetCachedVideos(req.GetLimit())
								if cachedVideos != nil {
									returnChanCache <- cachedVideos
									// return
								}
								return
							}
							errorChanUser <- status.Errorf(codes.Unavailable, "GetTopVideos: GetUser call failed: %v", err)
							return
						}
					} else {
						// atomic.AddUint64(server.TotalErrors, 1)
						if !server.options.DisableFallback {
							cachedVideos := server.GetCachedVideos(req.GetLimit())
							if cachedVideos != nil {
								returnChanCache <- cachedVideos
								// return
							}
							return
						}
						errorChanUser <- status.Errorf(codes.Unavailable, "GetTopVideos: GetUser call failed: %v", err)
						return
					}
				}
				// Append the result to the subscribedToUsers slice
				mu.Lock()
				subscribedToUsers = append(subscribedToUsers, subscribedToUsersResponse.GetUsers()...)
				mu.Unlock()

			}
			errorChanUser <- nil
		}(i)
	}

	go func() {
		wg.Wait()
		close(done)
	}()
	// println("reaching")
	select {
	case <-done:
	case cachedVideos := <-returnChanCache:
		atomic.AddUint64(server.StaleResponses, 1)
		atomic.AddUint64(server.TotalErrors, 1)

		rankedCachedVideos := rankVideos(subscriberUserInfo.GetUserCoefficients(), &cachedVideos, req.GetLimit())
		return &pb.GetTopVideosResponse{Videos: rankedCachedVideos, StaleResponse: true}, nil
	}

	select {
	case err := <-errorChanUser:
		if err != nil {
			atomic.AddUint64(server.TotalErrors, 1)
			return nil, err
		}
	default:
	}

	close(errorChanUser)

	likedVideos := make([]uint64, 0)
	LikedVideosMap := make(map[uint64]bool) // to make sure there are no duplicates

	for _, userInfo := range subscribedToUsers {
		vids := userInfo.GetLikedVideos()
		for _, v := range vids {
			if _, contains := LikedVideosMap[v]; !contains {
				LikedVideosMap[v] = true
				likedVideos = append(likedVideos, v)
			}
		}
	}
	// log.Printf("Liked videos: %v", likedVideos)

	// VideoServiceClient
	var videoClient vpb.VideoServiceClient
	if server.isMock {
		videoClient = server.mockVideoServiceClient
	} else {
		idx := atomic.AddUint64(server.VideoConnIndex, 1) % uint64(server.options.ClientPoolSize)
		videoClient = vpb.NewVideoServiceClient(server.VideoClientConn[idx]) // VideoServiceClient
	}
	// println("reaching 1")
	close(returnChanCache)
	maxVideoBatchSize := int(atomic.LoadUint64(server.VideoServiceMaxBatchSize)) // *server.VideoServiceMaxBatchSize)

	// returnChanCache = make(chan []*vpb.VideoInfo, maxVideoBatchSize+1)

	var videosInfo []*vpb.VideoInfo
	batchLoopVideo := len(likedVideos) / maxVideoBatchSize
	errorChanVideo := make(chan error, batchLoopVideo+1)
	returnChanCache = make(chan []*vpb.VideoInfo, batchLoopVideo+1)

	// var counter int32

	// go func() {
	// 	for {
	// 		time.Sleep(5000 * time.Millisecond)
	// 		fmt.Printf("Current active goroutines: %d\n", atomic.LoadInt32(&counter))
	// 	}
	// }()
	for i := 0; i <= batchLoopVideo; i++ {
		wg.Add(1)
		// atomic.AddInt32(&counter, 1)
		go func(i int) {
			defer wg.Done()
			// defer atomic.AddInt32(&counter, -1)
			if i == batchLoopVideo {
				if len(likedVideos)%maxVideoBatchSize != 0 {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					videosInfoResponse, err := videoClient.GetVideo(ctx, &vpb.GetVideoRequest{VideoIds: likedVideos[i*maxVideoBatchSize:]})
					if err != nil {
						// atomic.AddUint64(server.TotalErrors, 1)
						atomic.AddUint64(server.VideoServiceErrors, 1)
						if !server.options.DisableRetry {
							ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
							defer cancel()
							videosInfoResponse, err = videoClient.GetVideo(ctx, &vpb.GetVideoRequest{VideoIds: likedVideos[i*maxVideoBatchSize:]})
							if err != nil {
								// atomic.AddUint64(server.TotalErrors, 1)
								atomic.AddUint64(server.VideoServiceErrors, 1)
								if !server.options.DisableFallback {
									cachedVideos := server.GetCachedVideos(req.GetLimit())
									if cachedVideos != nil {
										returnChanCache <- cachedVideos
										// return
									}
									// println("reaching 3.1")
									return

								}
								errorChanVideo <- status.Errorf(codes.Unavailable, "GetTopVideos: GetVideo failed: %v", err)
								// println("reaching 3.2")

								return
							}
						} else {
							// atomic.AddUint64(server.TotalErrors, 1)
							if !server.options.DisableFallback {
								cachedVideos := server.GetCachedVideos(req.GetLimit())
								if cachedVideos != nil {
									returnChanCache <- cachedVideos
									// return
								}
								// println("reaching 3.3")

								return
							}
							errorChanVideo <- status.Errorf(codes.Unavailable, "GetTopVideos: GetVideo failed: %v", err)
							// println("reaching 3.4")

							return
						}
					}
					mu.Lock()
					videosInfo = append(videosInfo, videosInfoResponse.GetVideos()...)
					mu.Unlock()
				}
			} else {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				videosInfoResponse, err := videoClient.GetVideo(ctx, &vpb.GetVideoRequest{VideoIds: likedVideos[i*maxVideoBatchSize : (i+1)*maxVideoBatchSize]})
				if err != nil {
					// atomic.AddUint64(server.TotalErrors, 1)
					atomic.AddUint64(server.VideoServiceErrors, 1)
					if !server.options.DisableRetry {
						ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
						defer cancel()
						videosInfoResponse, err = videoClient.GetVideo(ctx, &vpb.GetVideoRequest{VideoIds: likedVideos[i*maxVideoBatchSize : (i+1)*maxVideoBatchSize]})
						if err != nil {
							// atomic.AddUint64(server.TotalErrors, 1)
							atomic.AddUint64(server.VideoServiceErrors, 1)
							if !server.options.DisableFallback {
								cachedVideos := server.GetCachedVideos(req.GetLimit())
								if cachedVideos != nil {
									// fmt.Printf("%+v\n", cachedVideos)
									returnChanCache <- cachedVideos
									// return
								}
								// println("reaching 3.5")

								return
							}
							errorChanVideo <- status.Errorf(codes.Unavailable, "GetTopVideos: GetVideo failed: %v", err)
							// println("reaching 3.6")

							return
						}
					} else {
						// atomic.AddUint64(server.TotalErrors, 1)
						if !server.options.DisableFallback {
							cachedVideos := server.GetCachedVideos(req.GetLimit())
							if cachedVideos != nil {
								returnChanCache <- cachedVideos
								// return
							}
							// println("reaching 3.7")

							return
						}
						errorChanVideo <- status.Errorf(codes.Unavailable, "GetTopVideos: GetVideo failed: %v", err)
						// println("reaching 3.8")

						return
					}
				}
				// println("reaching 3.9")

				mu.Lock()
				// println("reaching 3.10")

				videosInfo = append(videosInfo, videosInfoResponse.GetVideos()...)
				// println("reaching 3.11")

				mu.Unlock()
				// println("reaching 3.12")

			}
			errorChanVideo <- nil
		}(i)
	}
	// println("reaching 3")
	wg.Wait()
	// println("passed wg")
	select {
	case cachedVideos := <-returnChanCache:
		atomic.AddUint64(server.StaleResponses, 1)
		atomic.AddUint64(server.TotalErrors, 1)

		rankedCachedVideos := rankVideos(subscriberUserInfo.GetUserCoefficients(), &cachedVideos, req.GetLimit())

		return &pb.GetTopVideosResponse{Videos: rankedCachedVideos, StaleResponse: true}, nil
	default:
	}
	close(returnChanCache)
	// println("reaching 4")
	select {
	case err := <-errorChanVideo:
		if err != nil {
			atomic.AddUint64(server.TotalErrors, 1)
			return nil, err
		}
	default:
	}
	close(errorChanVideo)
	// println("reaching 5")
	// fmt.Println(subscriberUserInfo.GetUserCoefficients())
	likedVideosRanked := rankVideos(subscriberUserInfo.GetUserCoefficients(), &videosInfo, req.GetLimit())

	return &pb.GetTopVideosResponse{Videos: likedVideosRanked}, nil
}

func (server *VideoRecServiceServer) GetStats(
	ctx context.Context,
	req *pb.GetStatsRequest,
) (*pb.GetStatsResponse, error) {
	r := pb.GetStatsResponse{
		TotalRequests:      atomic.LoadUint64(server.TotalRequests),
		TotalErrors:        atomic.LoadUint64(server.TotalErrors),
		ActiveRequests:     atomic.LoadUint64(server.ActiveRequests),
		UserServiceErrors:  atomic.LoadUint64(server.UserServiceErrors),
		VideoServiceErrors: atomic.LoadUint64(server.VideoServiceErrors),
		AverageLatencyMs:   float32(atomic.LoadUint64(server.TotalSumLatencyMs) / max(1, atomic.LoadUint64(server.TotalRequests))),
		P99LatencyMs:       float32(server.td.Quantile(0.99)),
		StaleResponses:     atomic.LoadUint64(server.StaleResponses),
	}

	return &r, nil
}
