package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"

	mt5grpc "git.mtapi.io/root/grpc-proto.git/mt5/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func main() {
	config := &tls.Config{
		InsecureSkipVerify: false,
	}
	client, err := grpc.Dial("mt5grpc.mtapi.io:443", grpc.WithTransportCredentials(credentials.NewTLS(config)))
	if err != nil {
		log.Fatalf("failed to connect: %v", err)
	}
	defer client.Close()

	connection := mt5grpc.NewConnectionClient(client)
	mt5 := mt5grpc.NewMT5Client(client)
	//trading := mt5grpc.NewTradingClient(client)
	//service := mt5grpc.NewServiceClient(client)
	subscriptions := mt5grpc.NewSubscriptionsClient(client)
	streams := mt5grpc.NewStreamsClient(client)

	resp, err := connection.Connect(context.Background(), &mt5grpc.ConnectRequest{
		Host:     "78.140.180.198",
		Password: "tecimil4",
		Port:     443,
		User:     62333850,
	})
	if err != nil {
		log.Fatalf("gRPC protocol error: %v", err)
	}
	if resp.Error != nil {
		log.Fatalf("Server error: %v", resp.Error.Message)
	}
	fmt.Println(resp)
	id := resp.Result

	response, err := mt5.AccountSummary(context.Background(), &mt5grpc.AccountSummaryRequest{
		Id: id,
	})
	if err != nil {
		log.Fatalf("gRPC protocol error: %v", err)
	}
	if resp.Error != nil {
		log.Fatalf("Server error: %v", resp.Error.Message)
	}
	fmt.Println(response)

	r1, _ := subscriptions.Subscribe(context.Background(), &mt5grpc.SubscribeRequest{
		Id:       id,
		Interval: 0,
		Symbol:   "EURUSD",
	})
	fmt.Println(r1)

	stream, _ := streams.OnQuote(context.Background(), &mt5grpc.OnQuoteRequest{
		Id: id,
	})
	for {
		quote, _ := stream.Recv()
		fmt.Println(quote)
	}
}
