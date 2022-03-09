package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync/atomic"

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsublite/pscompat"
	influxdb "github.com/influxdata/influxdb-client-go/v2"
	_ "github.com/joho/godotenv/autoload"
	twitter "github.com/vniche/twitter-go"
)

func main() {
	projectID, ok := os.LookupEnv("GOOGLE_PROJECT_ID")
	if !ok {
		log.Fatalf("GOOGLE_PROJECT_ID env var is required")
	}

	zone, ok := os.LookupEnv("PUBSUB_ZONE")
	if !ok {
		log.Fatalf("PUBSUB_ZONE env var is required")
	}

	subscriptionID, ok := os.LookupEnv("PUBSUB_SUBSCRIPTION_ID")
	if !ok {
		log.Fatalf("PUBSUB_SUBSCRIPTION_ID env var is required")
	}

	influxToken, ok := os.LookupEnv("INFLUX_TOKEN")
	if !ok {
		log.Fatalf("INFLUX_TOKEN env var is required")
	}

	influxOrg, ok := os.LookupEnv("INFLUX_ORG")
	if !ok {
		log.Fatalf("INFLUX_ORG env var is required")
	}

	influxBucket, ok := os.LookupEnv("INFLUX_BUCKET")
	if !ok {
		log.Fatalf("INFLUX_BUCKET env var is required")
	}

	ctx := context.Background()

	// Create the subscriber client.
	subscriber, err := pscompat.NewSubscriberClientWithSettings(ctx,
		fmt.Sprintf("projects/%s/locations/%s/subscriptions/%s", projectID, zone, subscriptionID),
		pscompat.ReceiveSettings{
			MaxOutstandingBytes:    10 * 1024 * 1024,
			MaxOutstandingMessages: 1000,
		})
	if err != nil {
		log.Fatalf("pscompat.NewSubscriberClientWithSettings error: %v", err)
	}

	client := influxdb.NewClient("https://us-central1-1.gcp.cloud2.influxdata.com", influxToken)
	// always close client at the end
	defer client.Close()

	// get non-blocking write client
	writeAPI := client.WriteAPIBlocking(influxOrg, influxBucket)

	// Listen for messages until the timeout expires.
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var receiveCount int32

	// Receive blocks until the context is cancelled or an error occurs.
	if err := subscriber.Receive(cctx, func(ctx context.Context, msg *pubsub.Message) {
		// NOTE: May be called concurrently; synchronize access to shared memory.
		atomic.AddInt32(&receiveCount, 1)

		// Metadata decoded from the message ID contains the partition and offset.
		metadata, err := pscompat.ParseMessageMetadata(msg.ID)
		if err != nil {
			log.Fatalf("Failed to parse %q: %v", msg.ID, err)
		}

		fmt.Printf("Received (partition=%d, offset=%d): %s\n", metadata.Partition, metadata.Offset, string(msg.Data))

		var tweetStream *twitter.SearchStreamResponse
		err = json.Unmarshal(msg.Data, &tweetStream)
		if err != nil {
			log.Fatalf("Failed to unmarshal %q: %v", msg.ID, err)
		}

		p := influxdb.NewPoint(
			"tweets",
			map[string]string{
				"id":     tweetStream.Tweet.ID,
				"author": tweetStream.Includes.Users[0].Username,
			},
			map[string]interface{}{
				"followers_count": tweetStream.Includes.Users[0].PublicMetrics.FollowersCount,
			},
			tweetStream.Tweet.CreatedAt)
		if err = writeAPI.WritePoint(ctx, p); err != nil {
			log.Fatalf("Failed to publish metric %q: %v", msg.ID, err)
		}

		msg.Ack()
	}); err != nil {
		log.Fatalf("SubscriberClient.Receive error: %v", err)
	}

	fmt.Printf("Received %d messages\n", receiveCount)
}
