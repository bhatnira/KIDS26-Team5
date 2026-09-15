package tools

import (
	"context"
	"errors"

	"antelope/internal/modules/agent"

	"github.com/gin-gonic/gin"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// UserStatsGetter is the narrow contract the get_dashboard_stats tool needs
// from the dashboard service (mirrors the /user/dashboard/stats handler).
type UserStatsGetter interface {
	GetUserStats(userID uint) (gin.H, error)
}

type getDashboardStatsTool struct {
	dash UserStatsGetter
}

// NewGetDashboardStatsTool returns a tool that reports the requesting
// user's dashboard statistics (job counts by status, recent activity, …).
func NewGetDashboardStatsTool(dash UserStatsGetter) tool.CallableTool {
	return &getDashboardStatsTool{dash: dash}
}

func (t *getDashboardStatsTool) Declaration() *tool.Declaration {
	return &tool.Declaration{
		Name:        "get_dashboard_stats",
		Description: "Get the current user's dashboard statistics: job counts by status and recent platform activity.",
		InputSchema: &tool.Schema{
			Type:       "object",
			Properties: map[string]*tool.Schema{},
		},
	}
}

func (t *getDashboardStatsTool) Call(ctx context.Context, _ []byte) (any, error) {
	userID, ok := agent.UserIDFromContext(ctx)
	if !ok {
		return nil, errors.New("user id missing from context")
	}
	stats, err := t.dash.GetUserStats(userID)
	if err != nil {
		return map[string]any{"error": err.Error()}, nil
	}
	return stats, nil
}
