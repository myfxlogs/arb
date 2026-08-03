package dashboard

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"

	mt4 "arb/proto/gen/mtapi/mt4"
	mt5 "arb/proto/gen/mtapi/mt5"

	dashpb "arb/proto/gen/dashboard"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	mt4Gateway = "mt4grpc3.mtapi.io:443"
	mt5Gateway = "mt5grpc3.mtapi.io:443"
)

// SearchBroker searches MT4/MT5 brokers by company name via mtapi.io.
func (s *Server) SearchBroker(ctx context.Context, req *dashpb.SearchBrokerRequest) (*dashpb.SearchBrokerReply, error) {
	if req.Company == "" {
		return &dashpb.SearchBrokerReply{Error: "请输入经纪商名称"}, nil
	}

	var companies []*dashpb.SearchBrokerReply_BrokerCompany
	var searchErr error

	switch req.Platform {
	case 0: // MT4
		companies, searchErr = searchMT4(ctx, req.Company)
	case 1: // MT5
		companies, searchErr = searchMT5(ctx, req.Company)
	default:
		return &dashpb.SearchBrokerReply{Error: "未知平台类型"}, nil
	}

	if searchErr != nil {
		return &dashpb.SearchBrokerReply{Error: searchErr.Error()}, nil
	}

	return &dashpb.SearchBrokerReply{Companies: companies}, nil
}

func searchMT4(ctx context.Context, company string) ([]*dashpb.SearchBrokerReply_BrokerCompany, error) {
	conn, err := grpc.DialContext(ctx, mt4Gateway,
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})),
	)
	if err != nil {
		return nil, fmt.Errorf("连接 MT4 网关失败: %w", err)
	}
	defer conn.Close()

	svc := mt4.NewServiceClient(conn)
	reply, err := svc.Search(ctx, &mt4.SearchRequest{Company: company})
	if err != nil {
		return nil, fmt.Errorf("MT4 搜索失败: %w", err)
	}
	if reply.Error != nil {
		return nil, fmt.Errorf("MT4 搜索错误: %s", reply.Error.Message)
	}

	companies := make([]*dashpb.SearchBrokerReply_BrokerCompany, 0, len(reply.Result))
	for _, c := range reply.Result {
		servers := make([]*dashpb.SearchBrokerReply_BrokerServer, 0, len(c.Results))
		for _, r := range c.Results {
			access := ""
			if len(r.Access) > 0 {
				access = r.Access[0]
			}
			servers = append(servers, &dashpb.SearchBrokerReply_BrokerServer{
				Name:   r.Name,
				Access: access,
			})
		}
		companies = append(companies, &dashpb.SearchBrokerReply_BrokerCompany{
			CompanyName: c.CompanyName,
			Servers:     servers,
		})
	}
	return companies, nil
}

func searchMT5(ctx context.Context, company string) ([]*dashpb.SearchBrokerReply_BrokerCompany, error) {
	conn, err := grpc.DialContext(ctx, mt5Gateway,
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})),
	)
	if err != nil {
		return nil, fmt.Errorf("连接 MT5 网关失败: %w", err)
	}
	defer conn.Close()

	svc := mt5.NewServiceClient(conn)
	reply, err := svc.Search(ctx, &mt5.SearchRequest{Company: company})
	if err != nil {
		return nil, fmt.Errorf("MT5 搜索失败: %w", err)
	}
	if reply.Error != nil {
		return nil, fmt.Errorf("MT5 搜索错误: %s", reply.Error.Message)
	}

	companies := make([]*dashpb.SearchBrokerReply_BrokerCompany, 0, len(reply.Result))
	for _, c := range reply.Result {
		servers := make([]*dashpb.SearchBrokerReply_BrokerServer, 0, len(c.Results))
		for _, r := range c.Results {
			access := ""
			if len(r.Access) > 0 {
				access = strings.Join(r.Access, ", ")
			}
			servers = append(servers, &dashpb.SearchBrokerReply_BrokerServer{
				Name:   r.Name,
				Access: access,
			})
		}
		companies = append(companies, &dashpb.SearchBrokerReply_BrokerCompany{
			CompanyName: c.CompanyName,
			Servers:     servers,
		})
	}
	return companies, nil
}
