package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"

	mt4grpc "git.mtapi.io/root/grpc-proto.git/mt4/go"
	"github.com/davecgh/go-spew/spew"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func main() {
	config := &tls.Config{
		InsecureSkipVerify: false,
	}
	client, err := grpc.Dial("mt4grpc.mtapi.io:443", grpc.WithTransportCredentials(credentials.NewTLS(config)))
	if err != nil {
		log.Fatalf("failed to connect: %v", err)
	}
	defer client.Close()

	connection := mt4grpc.NewConnectionClient(client)
	mt4 := mt4grpc.NewMT4Client(client)
	//trading := mt4grpc.NewTradingClient(client)
	//service := mt4grpc.NewServiceClient(client)
	subscriptions := mt4grpc.NewSubscriptionsClient(client)
	streams := mt4grpc.NewStreamsClient(client)

	resp, err := connection.Connect(context.Background(), &mt4grpc.ConnectRequest{
		Host:     "mt4-demo.roboforex.com",
		Password: "ehj4bod",
		Port:     443,
		User:     500476959,
	})
	if err != nil {
		log.Fatalf("gRPC protocol error: %v", err)
	}
	if resp.Error != nil {
		log.Fatalf("Server error: %v", resp.Error.Message)
	}
	fmt.Println(resp)
	id := resp.Result

	response, err := mt4.AccountSummary(context.Background(), &mt4grpc.AccountSummaryRequest{
		Id: id,
	})
	if err != nil {
		log.Fatalf("gRPC protocol error: %v", err)
	}
	if resp.Error != nil {
		log.Fatalf("Server error: %v", resp.Error.Message)
	}
	fmt.Println(response)
	fmt.Println(response.Result.IsInvestor)
	spew.Dump(response.Result.IsInvestor)

	r1, _ := subscriptions.Subscribe(context.Background(), &mt4grpc.SubscribeRequest{
		Id:       id,
		Interval: 0,
		Symbol:   "EURUSD",
	})
	fmt.Println(r1)

	stream, _ := streams.OnQuote(context.Background(), &mt4grpc.OnQuoteRequest{
		Id: id,
	})
	for {
		quote, _ := stream.Recv()
		fmt.Println(quote)
	}
}
