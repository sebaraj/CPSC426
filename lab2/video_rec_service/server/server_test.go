package main

import (
	"context"
	// "fmt"
	// "math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	fipb "cs426.yale.edu/lab1/failure_injection/proto"
	umc "cs426.yale.edu/lab1/user_service/mock_client"
	usl "cs426.yale.edu/lab1/user_service/server_lib"
	vmc "cs426.yale.edu/lab1/video_service/mock_client"
	vsl "cs426.yale.edu/lab1/video_service/server_lib"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "cs426.yale.edu/lab1/video_rec_service/proto"
	sl "cs426.yale.edu/lab1/video_rec_service/server_lib"
)

func TestServerBasic(t *testing.T) {
	vrOptions := sl.VideoRecServiceOptions{
		MaxBatchSize:    50,
		DisableFallback: true,
		DisableRetry:    true,
	}
	// You can specify failure injection options here or later send
	// SetInjectionConfigRequests using these mock clients
	uClient := umc.MakeMockUserServiceClient(*usl.DefaultUserServiceOptions())
	vClient := vmc.MakeMockVideoServiceClient(*vsl.DefaultVideoServiceOptions())
	vrService := sl.MakeVideoRecServiceServerWithMocks(
		vrOptions,
		uClient,
		vClient,
	)

	var userId uint64 = 204054
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := vrService.GetTopVideos(
		ctx,
		&pb.GetTopVideosRequest{UserId: userId, Limit: 5},
	)
	assert.True(t, err == nil)

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	videos := out.Videos
	assert.Equal(t, 5, len(videos))
	assert.EqualValues(t, 1012, videos[0].VideoId)
	assert.Equal(t, "Harry Boehm", videos[1].Author)
	assert.EqualValues(t, 1209, videos[2].VideoId)
	assert.Equal(t, "https://video-data.localhost/blob/1309", videos[3].Url)
	assert.Equal(t, "precious here", videos[4].Title)

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	vClient.SetInjectionConfig(ctx, &fipb.SetInjectionConfigRequest{
		Config: &fipb.InjectionConfig{
			// fail one in 1 request, i.e., always fail
			FailureRate: 1,
		},
	})

	// Since we disabled retry and fallback, we expect the VideoRecService to
	// throw an error since the VideoService is "down".
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = vrService.GetTopVideos(
		ctx,
		&pb.GetTopVideosRequest{UserId: userId, Limit: 5},
	)
	assert.False(t, err == nil)
}

func TestServerFallbackDisabled(t *testing.T) {
	for i := 0; i < 2; i++ {
		vrOptions := sl.VideoRecServiceOptions{
			MaxBatchSize:    50,
			DisableFallback: true,
			DisableRetry:    i == 1,
		}
		uClient := umc.MakeMockUserServiceClient(*usl.DefaultUserServiceOptions())
		vClient := vmc.MakeMockVideoServiceClient(*vsl.DefaultVideoServiceOptions())
		vrService1 := sl.MakeVideoRecServiceServerWithMocks(
			vrOptions,
			uClient,
			vClient,
		)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := uClient.SetInjectionConfig(ctx, &fipb.SetInjectionConfigRequest{
			Config: &fipb.InjectionConfig{
				FailureRate: 1,
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		// test fallback when video service is down (note: user service is working, so it should returned ranked cache, i.e. the video order should be different)
		for i := 0; i < 10; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, err := vrService1.GetTopVideos(
				ctx,
				&pb.GetTopVideosRequest{UserId: uint64(204000 + i), Limit: 10},
			)
			assert.True(t, err != nil)

		}

		// check stats
		time.Sleep(100 * time.Millisecond)
		stats, err := vrService1.GetStats(context.Background(), &pb.GetStatsRequest{})
		assert.True(t, err == nil)
		// check if Get stats == vals in server and check that they are all 0
		assert.Equal(t, stats.TotalRequests, uint64(10))
		assert.Equal(t, stats.TotalErrors, uint64(10))
		assert.Equal(t, stats.ActiveRequests, uint64(0))
		assert.NotEqual(t, stats.UserServiceErrors, uint64(0))
		assert.Equal(t, stats.VideoServiceErrors, uint64(0))
		assert.Equal(t, stats.StaleResponses, uint64(0))

		assert.Equal(t, stats.TotalRequests, *vrService1.TotalRequests)
		assert.Equal(t, stats.TotalErrors, *vrService1.TotalErrors)
		assert.Equal(t, stats.ActiveRequests, *vrService1.ActiveRequests)
		assert.Equal(t, stats.UserServiceErrors, *vrService1.UserServiceErrors)
		assert.Equal(t, stats.VideoServiceErrors, *vrService1.VideoServiceErrors)
		assert.Equal(t, stats.StaleResponses, *vrService1.StaleResponses)
	}
}

func TestServerFallback(t *testing.T) {
	for i := 0; i < 2; i++ {
		vrOptions := sl.VideoRecServiceOptions{
			MaxBatchSize:    50,
			DisableFallback: false,
			DisableRetry:    i == 0,
		}
		uClient := umc.MakeMockUserServiceClient(*usl.DefaultUserServiceOptions())
		vClient := vmc.MakeMockVideoServiceClient(*vsl.DefaultVideoServiceOptions())
		vrService1 := sl.MakeVideoRecServiceServerWithMocks(
			vrOptions,
			uClient,
			vClient,
		)

		go vrService1.ContinuallyRefreshCache()

		// video service is "down" after cache has time to be populated, but before request to videorecservice is made
		time.Sleep(100 * time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var userId uint64 = 204054

		// test fallback when video service is down (note: user service is working, so it should returned ranked cache, i.e. the video order should be different)
		_, err := vClient.SetInjectionConfig(ctx, &fipb.SetInjectionConfigRequest{
			Config: &fipb.InjectionConfig{
				FailureRate: 1,
			},
		})
		if err != nil {
			// this shouldnt fail
			t.Fatal(err)
		}
		assert.True(t, err == nil)
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		videoRecServiceResponse, err := vrService1.GetTopVideos(
			ctx,
			&pb.GetTopVideosRequest{UserId: userId, Limit: 10},
		)
		assert.True(t, err == nil)

		videos := videoRecServiceResponse.Videos
		assert.Equal(t, 10, len(videos))

		// get cached videos from server itself (video service is down, so cache shouldnt updated, so no need to worry about atomicity)

		cacheVids := vrService1.TrendingCache
		cacheVidsSlice := cacheVids[:min(10, len(cacheVids))]
		assert.Equal(t, 10, len(cacheVidsSlice))
		assert.NotEqualValues(t, videos, cacheVidsSlice)

		// test fallback when user service is down
		_, err = uClient.SetInjectionConfig(ctx, &fipb.SetInjectionConfigRequest{
			Config: &fipb.InjectionConfig{
				FailureRate: 1,
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		videoRecServiceResponse2, err := vrService1.GetTopVideos(
			ctx,
			&pb.GetTopVideosRequest{UserId: userId, Limit: 10},
		)
		assert.True(t, err == nil)

		videos2 := videoRecServiceResponse2.Videos
		assert.Equal(t, 10, len(videos2))
		assert.EqualValues(t, videos2, cacheVidsSlice)
	}
}

func TestServerStatsWhenFailureFree(t *testing.T) {
	vrOptions := sl.VideoRecServiceOptions{
		MaxBatchSize:    50,
		DisableFallback: false,
		DisableRetry:    false,
	}
	uClient := umc.MakeMockUserServiceClient(*usl.DefaultUserServiceOptions())
	vClient := vmc.MakeMockVideoServiceClient(*vsl.DefaultVideoServiceOptions())
	vrService1 := sl.MakeVideoRecServiceServerWithMocks(
		vrOptions,
		uClient,
		vClient,
	)

	go vrService1.ContinuallyRefreshCache()

	// video service is "down" after cache has time to be populated, but before request to videorecservice is made
	time.Sleep(100 * time.Millisecond)

	// var userId uint64 = 204054

	// test fallback when video service is down (note: user service is working, so it should returned ranked cache, i.e. the video order should be different)
	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := vrService1.GetTopVideos(
			ctx,
			&pb.GetTopVideosRequest{UserId: uint64(204000 + i), Limit: 10},
		)
		assert.True(t, err == nil)

	}

	// check stats

	stats, err := vrService1.GetStats(context.Background(), &pb.GetStatsRequest{})
	assert.True(t, err == nil)
	// check if Get stats == vals in server and check that they are all 0
	assert.Equal(t, stats.TotalRequests, uint64(10))
	assert.Equal(t, stats.TotalErrors, uint64(0))
	assert.Equal(t, stats.ActiveRequests, uint64(0))
	assert.Equal(t, stats.UserServiceErrors, uint64(0))
	assert.Equal(t, stats.VideoServiceErrors, uint64(0))
	assert.Equal(t, stats.StaleResponses, uint64(0))

	assert.Equal(t, stats.TotalRequests, *vrService1.TotalRequests)
	assert.Equal(t, stats.TotalErrors, *vrService1.TotalErrors)
	assert.Equal(t, stats.ActiveRequests, *vrService1.ActiveRequests)
	assert.Equal(t, stats.UserServiceErrors, *vrService1.UserServiceErrors)
	assert.Equal(t, stats.VideoServiceErrors, *vrService1.VideoServiceErrors)
	assert.Equal(t, stats.StaleResponses, *vrService1.StaleResponses)
}

func TestServerStatsWhenFailureInjected(t *testing.T) {
	for retryEnabled := 0; retryEnabled < 2; retryEnabled++ {
		vrOptions := sl.VideoRecServiceOptions{
			MaxBatchSize:    50,
			DisableFallback: false,
			DisableRetry:    retryEnabled == 1,
		}
		uClient := umc.MakeMockUserServiceClient(*usl.DefaultUserServiceOptions())
		vClient := vmc.MakeMockVideoServiceClient(*vsl.DefaultVideoServiceOptions())
		vrService1 := sl.MakeVideoRecServiceServerWithMocks(
			vrOptions,
			uClient,
			vClient,
		)

		go vrService1.ContinuallyRefreshCache()

		// video service is "down" after cache has time to be populated, but before request to videorecservice is made
		time.Sleep(100 * time.Millisecond)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := vClient.SetInjectionConfig(ctx, &fipb.SetInjectionConfigRequest{
			Config: &fipb.InjectionConfig{
				FailureRate: 1,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		assert.True(t, err == nil)

		// test fallback when video service is down (note: user service is working, so it should returned ranked cache, i.e. the video order should be different)
		for i := 0; i < 10; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, err := vrService1.GetTopVideos(
				ctx,
				&pb.GetTopVideosRequest{UserId: uint64(204054), Limit: 10},
			)
			assert.True(t, err == nil)

		}

		// check stats

		stats, err := vrService1.GetStats(context.Background(), &pb.GetStatsRequest{})
		assert.True(t, err == nil)
		// check if Get stats == vals in server and check that they are all 0
		assert.Equal(t, stats.TotalRequests, uint64(10))
		assert.Equal(t, stats.TotalErrors, uint64(10))
		assert.Equal(t, stats.ActiveRequests, uint64(0))
		assert.Equal(t, stats.UserServiceErrors, uint64(0))
		// 10 requests * # failures per request (at least 7 batched attempts (specific for user 204054 at videorecservice max batch size 50), retried twice or once, depending on retry flag)
		assert.NotEqual(t, stats.VideoServiceErrors, uint64(0))

		//       if retryEnabled == 0 {
		// 	assert.GreaterOrEqual(t, stats.VideoServiceErrors, uint64(140))
		// } else {
		// 	assert.GreaterOrEqual(t, stats.VideoServiceErrors, uint64(70))
		// }
		assert.Equal(t, stats.StaleResponses, uint64(10))

		assert.Equal(t, stats.TotalRequests, *vrService1.TotalRequests)
		assert.Equal(t, stats.TotalErrors, *vrService1.TotalErrors)
		assert.Equal(t, stats.ActiveRequests, *vrService1.ActiveRequests)
		assert.Equal(t, stats.UserServiceErrors, *vrService1.UserServiceErrors)
		assert.Equal(t, stats.VideoServiceErrors, *vrService1.VideoServiceErrors)
		assert.Equal(t, stats.StaleResponses, *vrService1.StaleResponses)

		// spin down user service and check stats
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err = uClient.SetInjectionConfig(ctx, &fipb.SetInjectionConfigRequest{
			Config: &fipb.InjectionConfig{
				FailureRate: 1,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		assert.True(t, err == nil)
		for i := 0; i < 10; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, err := vrService1.GetTopVideos(
				ctx,
				&pb.GetTopVideosRequest{UserId: uint64(204054), Limit: 10},
			)
			assert.True(t, err == nil)

		}

		// check stats

		stats, err = vrService1.GetStats(context.Background(), &pb.GetStatsRequest{})
		assert.True(t, err == nil)
		// check if Get stats == vals in server and check that they are all 0
		assert.Equal(t, stats.TotalRequests, uint64(20))
		assert.Equal(t, stats.TotalErrors, uint64(20))
		assert.Equal(t, stats.ActiveRequests, uint64(0))
		// assert.Equal(t, stats.UserServiceErrors, uint64(0))
		assert.NotEqual(t, stats.UserServiceErrors, uint64(0))

		// if retryEnabled == 0 {
		// 	assert.GreaterOrEqual(t, stats.UserServiceErrors, uint64(20))
		// } else {
		// 	assert.GreaterOrEqual(t, stats.UserServiceErrors, uint64(10))
		// }
		assert.NotEqual(t, stats.VideoServiceErrors, uint64(0))

		// if retryEnabled == 0 {
		// 	assert.GreaterOrEqual(t, stats.VideoServiceErrors, uint64(140))
		// } else {
		// 	assert.GreaterOrEqual(t, stats.VideoServiceErrors, uint64(70))
		// }
		assert.Equal(t, stats.StaleResponses, uint64(20))

		assert.Equal(t, stats.TotalRequests, *vrService1.TotalRequests)
		assert.Equal(t, stats.TotalErrors, *vrService1.TotalErrors)
		assert.Equal(t, stats.ActiveRequests, *vrService1.ActiveRequests)
		assert.Equal(t, stats.UserServiceErrors, *vrService1.UserServiceErrors)
		assert.Equal(t, stats.VideoServiceErrors, *vrService1.VideoServiceErrors)
		assert.Equal(t, stats.StaleResponses, *vrService1.StaleResponses)

	}
}

func TestServerBatching(t *testing.T) {
	batchSizes := []int{-1, 0, 1, 10, 50}
	for _, batchSize := range batchSizes {
		vrOptions := sl.VideoRecServiceOptions{
			MaxBatchSize:    batchSize,
			DisableFallback: false,
			DisableRetry:    false,
		}
		uClient := umc.MakeMockUserServiceClient(*usl.DefaultUserServiceOptions())
		vClient := vmc.MakeMockVideoServiceClient(*vsl.DefaultVideoServiceOptions())
		vrService1 := sl.MakeVideoRecServiceServerWithMocks(
			vrOptions,
			uClient,
			vClient,
		)

		go vrService1.ContinuallyRefreshCache()

		// video service is "down" after cache has time to be populated, but before request to videorecservice is made
		time.Sleep(100 * time.Millisecond)

		// var userId uint64 = 204054

		// test fallback when video service is down (note: user service is working, so it should returned ranked cache, i.e. the video order should be different)
		for i := 0; i < 10; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, err := vrService1.GetTopVideos(
				ctx,
				&pb.GetTopVideosRequest{UserId: uint64(204000 + i), Limit: 10},
			)
			assert.True(t, err == nil)

		}

		// check stats

		stats, err := vrService1.GetStats(context.Background(), &pb.GetStatsRequest{})
		assert.True(t, err == nil)
		// check if Get stats == vals in server and check that they are all 0
		assert.Equal(t, stats.TotalRequests, uint64(10))
		assert.Equal(t, stats.TotalErrors, uint64(0))
		assert.Equal(t, stats.ActiveRequests, uint64(0))
		assert.Equal(t, stats.UserServiceErrors, uint64(0))
		assert.Equal(t, stats.VideoServiceErrors, uint64(0))
		assert.Equal(t, stats.StaleResponses, uint64(0))

		assert.Equal(t, stats.TotalRequests, *vrService1.TotalRequests)
		assert.Equal(t, stats.TotalErrors, *vrService1.TotalErrors)
		assert.Equal(t, stats.ActiveRequests, *vrService1.ActiveRequests)
		assert.Equal(t, stats.UserServiceErrors, *vrService1.UserServiceErrors)
		assert.Equal(t, stats.VideoServiceErrors, *vrService1.VideoServiceErrors)
		assert.Equal(t, stats.StaleResponses, *vrService1.StaleResponses)
	}
}

func TestDifferentServiceBatchingMax(t *testing.T) {
	userServiceMaxBatchSizes := []int{1, 5, 10, 50, 100}
	videoServiceMaxBatchSizes := []int{1, 4, 9, 50, 99}
	for i := 0; i < 10; i++ {
		vrOptions := sl.VideoRecServiceOptions{
			MaxBatchSize:    50,
			DisableFallback: false,
			DisableRetry:    false,
		}
		userOptions := usl.DefaultUserServiceOptions()
		userOptions.MaxBatchSize = userServiceMaxBatchSizes[i/len(userServiceMaxBatchSizes)]
		uClient := umc.MakeMockUserServiceClient(*userOptions)
		videoOptions := vsl.DefaultVideoServiceOptions()
		videoOptions.MaxBatchSize = videoServiceMaxBatchSizes[i/len(videoServiceMaxBatchSizes)]
		vClient := vmc.MakeMockVideoServiceClient(*videoOptions)
		vrService1 := sl.MakeVideoRecServiceServerWithMocks(
			vrOptions,
			uClient,
			vClient,
		)
		if i%2 == 1 {
			// inject failures
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, err := vClient.SetInjectionConfig(ctx, &fipb.SetInjectionConfigRequest{
				Config: &fipb.InjectionConfig{
					// fail one in 1 request, i.e., always fail
					FailureRate: 10,
				},
			})
			if err != nil {
				// this shouldnt fail
				t.Fatal(err)
			}

			ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, err = uClient.SetInjectionConfig(ctx, &fipb.SetInjectionConfigRequest{
				Config: &fipb.InjectionConfig{
					// fail one in 1 request, i.e., always fail
					FailureRate: 10,
				},
			})
			if err != nil {
				// this shouldnt fail
				t.Fatal(err)
			}

		}
		go vrService1.ContinuallyRefreshCache()

		// video service is "down" after cache has time to be populated, but before request to videorecservice is made
		time.Sleep(100 * time.Millisecond)
		// cache := vrService1.TrendingCache
		// fmt.Println(cache)
		var userId uint64 = 204054

		for i := 0; i < 10; i++ {
			// println("starting")
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, _ = vrService1.GetTopVideos(
				ctx,
				&pb.GetTopVideosRequest{UserId: uint64(userId), Limit: 10},
			)
			assert.NotEqual(t, ctx.Err(), context.DeadlineExceeded)
			// println("done")
			// if err != nil {
			// println(err)
			// }
			// assert.True(t, err == nil)

		}
		stats, err := vrService1.GetStats(context.Background(), &pb.GetStatsRequest{})
		assert.True(t, err == nil)
		// check if Get stats == vals in server and check that they are all 0

		assert.Equal(t, *vrService1.UserServiceMaxBatchSize, uint64(userServiceMaxBatchSizes[i/len(userServiceMaxBatchSizes)]))
		assert.Equal(t, *vrService1.VideoServiceMaxBatchSize, uint64(videoServiceMaxBatchSizes[i/len(videoServiceMaxBatchSizes)]))

		assert.Equal(t, stats.TotalRequests, uint64(10))
		assert.Equal(t, stats.ActiveRequests, uint64(0))

		assert.Equal(t, stats.TotalRequests, *vrService1.TotalRequests)
		assert.Equal(t, stats.ActiveRequests, *vrService1.ActiveRequests)

	}
}

func TestServerReturnedErrors(t *testing.T) {
	// get top videos (cache is disabled, retry is enabled) when user service is disabled. check that error is code.Internal (i.e. is down)
	vrOptions1 := sl.VideoRecServiceOptions{
		MaxBatchSize:    50,
		DisableFallback: true,
		DisableRetry:    false,
	}
	uClient1 := umc.MakeMockUserServiceClient(*usl.DefaultUserServiceOptions())
	vClient1 := vmc.MakeMockVideoServiceClient(*vsl.DefaultVideoServiceOptions())
	vrService1 := sl.MakeVideoRecServiceServerWithMocks(
		vrOptions1,
		uClient1,
		vClient1,
	)
	var userId uint64 = 204054

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := uClient1.SetInjectionConfig(ctx, &fipb.SetInjectionConfigRequest{
		Config: &fipb.InjectionConfig{
			FailureRate: 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = vrService1.GetTopVideos(
		ctx,
		&pb.GetTopVideosRequest{UserId: uint64(userId), Limit: 10},
	)
	assert.NotEqual(t, ctx.Err(), context.DeadlineExceeded)
	s, ok := status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, s.Code(), codes.Internal)

	// ^^ but when cache is enabled, check that there is no error when cache is population
	vrOptions2 := sl.VideoRecServiceOptions{
		MaxBatchSize:    50,
		DisableFallback: false,
		DisableRetry:    false,
	}
	uClient2 := umc.MakeMockUserServiceClient(*usl.DefaultUserServiceOptions())
	vClient2 := vmc.MakeMockVideoServiceClient(*vsl.DefaultVideoServiceOptions())
	vrService2 := sl.MakeVideoRecServiceServerWithMocks(
		vrOptions2,
		uClient2,
		vClient2,
	)
	go vrService2.ContinuallyRefreshCache()
	time.Sleep(100 * time.Millisecond)

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = uClient2.SetInjectionConfig(ctx, &fipb.SetInjectionConfigRequest{
		Config: &fipb.InjectionConfig{
			FailureRate: 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = vrService2.GetTopVideos(
		ctx,
		&pb.GetTopVideosRequest{UserId: uint64(userId), Limit: 10},
	)
	assert.NotEqual(t, ctx.Err(), context.DeadlineExceeded)
	assert.True(t, err == nil)

	// ^^ check that there is an error when cache is empty
	vrOptions3 := sl.VideoRecServiceOptions{
		MaxBatchSize:    50,
		DisableFallback: false,
		DisableRetry:    false,
	}
	uClient3 := umc.MakeMockUserServiceClient(*usl.DefaultUserServiceOptions())
	vClient3 := vmc.MakeMockVideoServiceClient(*vsl.DefaultVideoServiceOptions())
	vrService3 := sl.MakeVideoRecServiceServerWithMocks(
		vrOptions3,
		uClient3,
		vClient3,
	)

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = uClient3.SetInjectionConfig(ctx, &fipb.SetInjectionConfigRequest{
		Config: &fipb.InjectionConfig{
			FailureRate: 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	response3, err := vrService3.GetTopVideos(
		ctx,
		&pb.GetTopVideosRequest{UserId: uint64(userId), Limit: 10},
	)
	assert.NotEqual(t, ctx.Err(), context.DeadlineExceeded)
	assert.True(t, response3.GetVideos() == nil)
	s, ok = status.FromError(err)
	// returns internal error because user service is down/cache is empty
	assert.True(t, ok)
	assert.Equal(t, s.Code(), codes.Internal)

	// get top videos when user service works, but video service is disabled. check thst error is code.Unavailable (i.e. is down)
	vrOptions4 := sl.VideoRecServiceOptions{
		MaxBatchSize:    50,
		DisableFallback: true,
		DisableRetry:    false,
	}
	uClient4 := umc.MakeMockUserServiceClient(*usl.DefaultUserServiceOptions())
	vClient4 := vmc.MakeMockVideoServiceClient(*vsl.DefaultVideoServiceOptions())
	vrService4 := sl.MakeVideoRecServiceServerWithMocks(
		vrOptions4,
		uClient4,
		vClient4,
	)

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = vClient4.SetInjectionConfig(ctx, &fipb.SetInjectionConfigRequest{
		Config: &fipb.InjectionConfig{
			FailureRate: 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = vrService4.GetTopVideos(
		ctx,
		&pb.GetTopVideosRequest{UserId: uint64(userId), Limit: 10},
	)
	assert.NotEqual(t, ctx.Err(), context.DeadlineExceeded)
	s, ok = status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, s.Code(), codes.Unavailable)

	// get top videos when userID = 0; should return code.InvalidArgument
	vrOptions7 := sl.VideoRecServiceOptions{
		MaxBatchSize:    50,
		DisableFallback: true,
		DisableRetry:    false,
	}
	uClient7 := umc.MakeMockUserServiceClient(*usl.DefaultUserServiceOptions())
	vClient7 := vmc.MakeMockVideoServiceClient(*vsl.DefaultVideoServiceOptions())
	vrService7 := sl.MakeVideoRecServiceServerWithMocks(
		vrOptions7,
		uClient7,
		vClient7,
	)

	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	response7, err := vrService7.GetTopVideos(
		ctx,
		&pb.GetTopVideosRequest{UserId: uint64(0), Limit: 10},
	)
	assert.NotEqual(t, ctx.Err(), context.DeadlineExceeded)
	assert.True(t, response7.GetVideos() == nil)
	s, ok = status.FromError(err)
	assert.True(t, ok)
	assert.Equal(t, s.Code(), codes.InvalidArgument)
}
