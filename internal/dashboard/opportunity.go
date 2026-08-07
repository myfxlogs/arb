package dashboard

import (
	"context"
	"log/slog"
	"time"

	"arb/internal/engine"
	"arb/internal/evaluator"

	dashpb "arb/proto/gen/dashboard"
)

// OpportunityStream streams opportunity events to the desk client.
// Subscribes to the engine's event channel and forwards each event.
func (s *Server) OpportunityStream(
	req *dashpb.OpportunityStreamRequest,
	stream dashpb.DashboardService_OpportunityStreamServer,
) error {
	if s.engine == nil {
		slog.Warn("opportunity_stream: engine not configured")
		return nil
	}

	ctx := stream.Context()
	ch, cancel := s.engine.Subscribe()
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			pbEv := toProtoEvent(ev)
			if err := stream.Send(pbEv); err != nil {
				return err
			}
		}
	}
}

// ConfirmOpportunity handles desk → core confirmation of an opportunity.
func (s *Server) ConfirmOpportunity(
	ctx context.Context,
	req *dashpb.ConfirmRequest,
) (*dashpb.ConfirmReply, error) {
	if s.engine == nil {
		return &dashpb.ConfirmReply{Accepted: false, Reason: "engine not configured"}, nil
	}

	opp, reason := s.engine.ConfirmOpportunity(req.GetOpportunityId())
	if opp == nil {
		return &dashpb.ConfirmReply{Accepted: false, Reason: reason}, nil
	}

	return &dashpb.ConfirmReply{Accepted: true}, nil
}

func toProtoEvent(ev engine.OpportunityEvent) *dashpb.OpportunityEvent {
	pbOpp := toProtoOpp(ev.Opp)
	var action dashpb.OpportunityAction
	switch ev.Action {
	case "PUSHED":
		action = dashpb.OpportunityAction_OPP_ACTION_PUSHED
	case "EXPIRED":
		action = dashpb.OpportunityAction_OPP_ACTION_EXPIRED
	default:
		action = dashpb.OpportunityAction_OPP_ACTION_UPDATED
	}
	return &dashpb.OpportunityEvent{
		Id:              pbOpp.Id,
		Opp:             pbOpp,
		Action:          action,
		TimestampUnixMs: time.Now().UnixMilli(),
		Reason:          ev.Reason,
	}
}

func toProtoOpp(opp *evaluator.Opportunity) *dashpb.Opportunity {
	if opp == nil {
		return &dashpb.Opportunity{}
	}
	return &dashpb.Opportunity{
		Id:               opp.ID,
		Type:             toProtoOppType(opp.Type),
		Legs:             toProtoLegs(opp.Legs),
		QuoteTimeUnixMs:  opp.QuoteTime.UnixMilli(),
		GrossProfit:      opp.GrossProfit.String(),
		SpreadCost:       opp.SpreadCost.String(),
		CommissionCost:   opp.CommissionCost.String(),
		SlippageCost:     opp.SlippageCost.String(),
		SwapCost:         opp.SwapCost.String(),
		NetProfit:        opp.NetProfit.String(),
		NetBps:           opp.NetBps.String(),
		NetSwapPerDay:    opp.NetSwapPerDay.String(),
		HoldDaysHint:     opp.HoldDaysHint,
		AnnualizedNetBps: opp.AnnualizedNetBps.String(),
		ExpiresAtUnixMs:  opp.ExpiresAt.UnixMilli(),
		Executable:       opp.Executable,
		Confidence:       opp.Confidence,
		Status:           toProtoOppStatus(opp.Status),
		NotionalUsd:      opp.NotionalUSD.String(),
		RejectReason:     opp.RejectReason,
	}
}

func toProtoLegs(legs []evaluator.OppLeg) []*dashpb.Leg {
	out := make([]*dashpb.Leg, 0, len(legs))
	for _, leg := range legs {
		out = append(out, &dashpb.Leg{
			Broker:          leg.Broker,
			BrokerSymbol:    leg.BrokerSymbol,
			CanonicalSymbol: leg.Canonical,
			Direction:       toProtoBuySell(leg.Direction),
			Lots:            leg.Lots.String(),
			EstimatePrice:   leg.EstPrice.String(),
			Role:            toProtoLegRole(leg.Role),
			DailySwap:       leg.DailySwap.String(),
			AnnualizedBps:   leg.AnnualizedBps.String(),
		})
	}
	return out
}

func toProtoOppType(t evaluator.OppType) dashpb.OppType {
	switch t {
	case evaluator.OppCrossExchange:
		return dashpb.OppType_OPP_TYPE_CROSS_EXCHANGE
	case evaluator.OppCarry:
		return dashpb.OppType_OPP_TYPE_CARRY
	case evaluator.OppTriangular:
		return dashpb.OppType_OPP_TYPE_TRIANGULAR
	default:
		return dashpb.OppType_OPP_TYPE_UNSPECIFIED
	}
}

func toProtoBuySell(d evaluator.BuySell) dashpb.BuySell {
	switch d {
	case evaluator.Buy:
		return dashpb.BuySell_BUY_SELL_BUY
	case evaluator.Sell:
		return dashpb.BuySell_BUY_SELL_SELL
	default:
		return dashpb.BuySell_BUY_SELL_UNSPECIFIED
	}
}

func toProtoLegRole(r evaluator.LegRole) dashpb.LegRole {
	switch r {
	case evaluator.LegRoleIncome:
		return dashpb.LegRole_LEG_ROLE_INCOME
	case evaluator.LegRoleHedge:
		return dashpb.LegRole_LEG_ROLE_HEDGE
	default:
		return dashpb.LegRole_LEG_ROLE_UNSPECIFIED
	}
}

func toProtoOppStatus(s evaluator.OppStatus) dashpb.OppStatus {
	switch s {
	case evaluator.OppStatusPushed:
		return dashpb.OppStatus_OPP_STATUS_PUSHED
	case evaluator.OppStatusConfirmed:
		return dashpb.OppStatus_OPP_STATUS_CONFIRMED
	case evaluator.OppStatusExecuting:
		return dashpb.OppStatus_OPP_STATUS_EXECUTING
	case evaluator.OppStatusFilled:
		return dashpb.OppStatus_OPP_STATUS_FILLED
	case evaluator.OppStatusFailed:
		return dashpb.OppStatus_OPP_STATUS_FAILED
	case evaluator.OppStatusExpired:
		return dashpb.OppStatus_OPP_STATUS_EXPIRED
	default:
		return dashpb.OppStatus_OPP_STATUS_UNSPECIFIED
	}
}
