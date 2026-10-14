package trazrextension

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/gorilla/websocket"
	"github.com/medxops/trazr-otel/pkg/auth"
	"github.com/medxops/trazr-otel/trazrextension/internal"
)

// Recheck long-lived subscriptions as well as the initial HTTP handshake.
// Privilege changes close the stream so its captured filters cannot stay stale.
func (h *TrazrExtension) guardWebSocketSession(ctx context.Context, cancel context.CancelFunc, r *http.Request, conn *websocket.Conn) {
	initial, err := auth.GetUserContext(r.Context())
	if err != nil {
		cancel()
		_ = conn.Close()
		return
	}
	roles, schemas := slices.Clone(initial.Roles), slices.Clone(initial.Schemas)
	scopes := slices.Clone(initial.Scopes)
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				checkCtx, stop := context.WithTimeout(ctx, 5*time.Second)
				claims, err := auth.ValidateJWTFromRequest(r.WithContext(checkCtx))
				stop()
				if err != nil || claims.UserID != initial.UserID || claims.TenantID != initial.TenantID ||
					!sameOptionalID(claims.OrgID, initial.OrgID) || !sameOptionalID(claims.ProjectID, initial.ProjectID) ||
					!slices.Equal(claims.Roles, roles) || !slices.Equal(claims.Schemas, schemas) || !slices.Equal(claims.Scopes, scopes) {
					cancel()
					_ = internal.SafeWriteControl(conn, websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "session access changed"), time.Now().Add(time.Second))
					_ = conn.Close()
					return
				}
			}
		}
	}()
}

func sameOptionalID(a, b *int64) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}
