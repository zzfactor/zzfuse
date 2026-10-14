package trazrextension

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/medxops/trazr-otel/pkg/event"
	"github.com/medxops/trazr-otel/pkg/repository"
	"github.com/medxops/trazr-otel/trazrextension/internal"

	"github.com/gorilla/websocket"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/medxops/trazr-otel/pkg/auth"
	pgstore "github.com/medxops/trazr-otel/pkg/auth/storage/pg"
	"github.com/medxops/trazr-otel/pkg/util"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
)

// WebSocket default constants
const (
	defaultWSReadTimeout          = 120 * time.Second
	defaultWSReadLimit      int64 = 1 << 20 // ~1MB
	serverPingWriteDeadline       = 5 * time.Second
	wsReadBufferSize              = 1024
	wsWriteBufferSize             = 1024
)

// newWSUpgrader creates a WebSocket upgrader with origin checking based on allowed origins.
func newWSUpgrader(allowedOrigins []string) websocket.Upgrader {
	return websocket.Upgrader{
		ReadBufferSize:  wsReadBufferSize,
		WriteBufferSize: wsWriteBufferSize,
		CheckOrigin:     makeOriginChecker(allowedOrigins),
	}
}

// makeOriginChecker returns an origin-checking function based on the allowed origins list.
// If the list is empty or contains "*", all origins are allowed (dev compatibility).
func makeOriginChecker(allowedOrigins []string) func(r *http.Request) bool {
	if len(allowedOrigins) == 0 {
		return func(r *http.Request) bool { return true }
	}
	for _, o := range allowedOrigins {
		if o == "*" {
			return func(r *http.Request) bool { return true }
		}
	}
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[strings.ToLower(strings.TrimSpace(o))] = struct{}{}
	}
	return func(r *http.Request) bool {
		origin := strings.ToLower(strings.TrimSpace(r.Header.Get("Origin")))
		if origin == "" {
			return true // non-browser clients may not send Origin
		}
		_, ok := allowed[origin]
		return ok
	}
}

const TrazrExtensionType = "trazr"

//go:embed openapi.yaml
var openapiMergedYAML []byte

// TrazrExtension is an OTel Collector extension that exposes a REST/WebSocket API
// and owns the ServiceResourceManager. It implements repository.TrazrServiceProvider
// to allow exporters to access the shared SRM via Host.GetExtensions().
type TrazrExtension struct {
	server          *http.Server
	logger          *zap.Logger
	tracer          trace.Tracer
	rsMgr           repository.ServiceResourceManagerInterface
	cfg             *Config
	wsUpgrader      websocket.Upgrader
	resetDispatcher *auth.PasswordResetDispatcher

	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
	wsConns        sync.Map // map[*websocket.Conn]struct{}

	// ready is closed when the ServiceResourceManager is fully initialized
	ready chan struct{}
	// mu protects rsMgr during initialization
	mu sync.RWMutex
}

var _ extension.Extension = (*TrazrExtension)(nil)

// Ensure TrazrExtension implements TrazrServiceProvider
var _ repository.TrazrServiceProvider = (*TrazrExtension)(nil)

// withCORS is a CORS middleware for HTTP handlers.
// If allowedOrigins is empty or contains "*", all origins are allowed (dev compatibility).
// Otherwise, only requests from listed origins receive CORS headers.
func withCORS(allowedOrigins []string, next http.Handler) http.Handler {
	allowAll := len(allowedOrigins) == 0
	if !allowAll {
		for _, o := range allowedOrigins {
			if o == "*" {
				allowAll = true
				break
			}
		}
	}

	var allowed map[string]struct{}
	if !allowAll {
		allowed = make(map[string]struct{}, len(allowedOrigins))
		for _, o := range allowedOrigins {
			allowed[strings.ToLower(strings.TrimSpace(o))] = struct{}{}
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if allowAll {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else if origin != "" {
			if _, ok := allowed[strings.ToLower(origin)]; ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// withSecurityHeaders adds standard security headers to HTTP responses.
func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// JWTAuthMiddleware wraps an http.HandlerFunc to require a valid JWT.
func JWTAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return auth.UserContextMiddleware(next).ServeHTTP
}

// AdminOnlyMiddleware validates JWT and enforces admin role. Returns 401 for invalid/missing token,
// and 403 Forbidden for valid non-admin tokens.
func AdminOnlyMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return auth.UserContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !auth.UserHasRole(r.Context(), "admin") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "forbidden: admin required"})
			return
		}
		next(w, r)
	})).ServeHTTP
}

// wsAuthHandlerWrapper authenticates JWT for WebSocket endpoints without org/project enforcement
func (h *TrazrExtension) wsAuthHandlerWrapper(handler func(w http.ResponseWriter, r *http.Request, conn *websocket.Conn)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, err := auth.ValidateJWTFromRequest(r)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		r = r.WithContext(auth.WithUserContext(r.Context(), auth.ClaimsToUserContext(claims)))
		conn, err := h.wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, "websocket upgrade failed: "+err.Error(), http.StatusBadRequest)
			return
		}
		h.registerWS(conn)
		internal.RegisterWSSafe(conn)
		// Register connection with rate limiter
		internal.GetWSRateLimiter().RegisterConnection(conn)
		defer func() {
			// Unregister from rate limiter
			internal.GetWSRateLimiter().UnregisterConnection(conn)
			internal.UnregisterWSSafe(conn)
			conn.Close()
			h.unregisterWS(conn)
		}()
		readTimeout := h.cfg.WSReadTimeout
		if readTimeout == 0 {
			readTimeout = defaultWSReadTimeout
		}
		readLimit := h.cfg.WSReadLimit
		if readLimit == 0 {
			readLimit = defaultWSReadLimit
		}
		pingInterval := h.cfg.WSPingInterval

		conn.SetReadLimit(readLimit)
		_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
		conn.SetPongHandler(func(string) error {
			h.logger.Debug("Pong received; extending read deadline", zap.Duration("timeout", readTimeout))
			return conn.SetReadDeadline(time.Now().Add(readTimeout))
		})
		defaultPingHandler := conn.PingHandler()
		conn.SetPingHandler(func(appData string) error {
			h.logger.Debug("Ping received; extending read deadline", zap.Duration("timeout", readTimeout))
			if err := defaultPingHandler(appData); err != nil {
				return err
			}
			return conn.SetReadDeadline(time.Now().Add(readTimeout))
		})

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		go func() {
			select {
			case <-h.shutdownCtx.Done():
				cancel()
			case <-ctx.Done():
			}
		}()
		h.guardWebSocketSession(ctx, cancel, r, conn)
		// Note: Do not start an additional read-drain here. Handlers wrapped by
		// wsScopedAuthHandlerWrapper (e.g., StreamLogs) maintain their own
		// reader goroutines for control/data frames. Running a second reader causes
		// concurrent reads on the same WebSocket connection, which is unsupported
		// by Gorilla and can lead to missed Ping/Pong handling and timeouts.
		if pingInterval > 0 {
			go func() {
				ticker := time.NewTicker(pingInterval)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						deadline := time.Now().Add(serverPingWriteDeadline)
						h.logger.Debug("Sending server Ping")
						_ = internal.SafeWriteControl(conn, websocket.PingMessage, nil, deadline)
					}
				}
			}()
		}
		handler(w, r.WithContext(ctx), conn)
	}
}

// wsSchemaAuthHandlerWrapper enforces the requested schema before upgrading a signal stream.
func (h *TrazrExtension) wsSchemaAuthHandlerWrapper(handler func(w http.ResponseWriter, r *http.Request, conn *websocket.Conn)) http.HandlerFunc {
	wrapped := internal.SchemaAuthMiddleware(h.wsScopedAuthHandlerWrapper(handler))
	return auth.UserContextMiddleware(wrapped).ServeHTTP
}

// wsScopedAuthHandlerWrapper validates JWT and enforces org/project scoping for WebSocket endpoints
func (h *TrazrExtension) wsScopedAuthHandlerWrapper(handler func(w http.ResponseWriter, r *http.Request, conn *websocket.Conn)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, err := auth.ValidateJWTFromRequest(r)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized", h.logger)
			return
		}
		r = r.WithContext(auth.WithUserContext(r.Context(), auth.ClaimsToUserContext(claims)))
		checker := auth.ScopedJWTAuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
			// no-op, proceed to upgrade
		})
		rr := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		checker(rr, r)
		if rr.status != http.StatusOK {
			return // error already written
		}
		conn, err := h.wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, "websocket upgrade failed: "+err.Error(), http.StatusBadRequest)
			return
		}
		h.registerWS(conn)
		internal.RegisterWSSafe(conn)
		// Register connection with rate limiter
		internal.GetWSRateLimiter().RegisterConnection(conn)
		defer func() {
			// Unregister from rate limiter
			internal.GetWSRateLimiter().UnregisterConnection(conn)
			internal.UnregisterWSSafe(conn)
			conn.Close()
			h.unregisterWS(conn)
		}()
		readTimeout := h.cfg.WSReadTimeout
		if readTimeout == 0 {
			readTimeout = defaultWSReadTimeout
		}
		readLimit := h.cfg.WSReadLimit
		if readLimit == 0 {
			readLimit = 1 << 20 // ~1MB
		}
		pingInterval := h.cfg.WSPingInterval

		conn.SetReadLimit(readLimit)
		_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
		conn.SetPongHandler(func(string) error {
			return conn.SetReadDeadline(time.Now().Add(readTimeout))
		})
		defaultPingHandler2 := conn.PingHandler()
		conn.SetPingHandler(func(appData string) error {
			if err := defaultPingHandler2(appData); err != nil {
				return err
			}
			return conn.SetReadDeadline(time.Now().Add(readTimeout))
		})

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		go func() {
			select {
			case <-h.shutdownCtx.Done():
				cancel()
			case <-ctx.Done():
			}
		}()
		h.guardWebSocketSession(ctx, cancel, r, conn)
		// Note: Do not start an additional read-drain here. Handlers wrapped by
		// wsScopedAuthHandlerWrapper (e.g., StreamLogs) maintain their own
		// reader goroutines. A second reader on the same connection is unsupported
		// and can break Ping/Pong handling.
		if pingInterval > 0 {
			go func() {
				ticker := time.NewTicker(pingInterval)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						deadline := time.Now().Add(5 * time.Second)
						h.logger.Debug("Sending server Ping")
						_ = internal.SafeWriteControl(conn, websocket.PingMessage, nil, deadline)
					}
				}
			}()
		}
		handler(w, r.WithContext(ctx), conn)
	}
}

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (rr *responseRecorder) WriteHeader(statusCode int) {
	rr.status = statusCode
	rr.ResponseWriter.WriteHeader(statusCode)
}

func (h *TrazrExtension) registerWS(conn *websocket.Conn) {
	h.wsConns.Store(conn, struct{}{})
}

func (h *TrazrExtension) unregisterWS(conn *websocket.Conn) {
	h.wsConns.Delete(conn)
}

// GetServiceResourceManager returns the shared ServiceResourceManager instance.
// Implements repository.TrazrServiceProvider.
func (h *TrazrExtension) GetServiceResourceManager() (repository.ServiceResourceManagerInterface, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.rsMgr == nil {
		return nil, fmt.Errorf("service resource manager not initialized")
	}
	return h.rsMgr, nil
}

// WaitForReady blocks until the TrazrExtension has fully initialized its services.
// Implements repository.TrazrServiceProvider.
func (h *TrazrExtension) WaitForReady(ctx context.Context) error {
	select {
	case <-h.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// IsReady returns true if the TrazrExtension has fully initialized its services.
// Implements repository.TrazrServiceProvider.
func (h *TrazrExtension) IsReady() bool {
	select {
	case <-h.ready:
		return true
	default:
		return false
	}
}

func (h *TrazrExtension) Start(ctx context.Context, _ component.Host) error {
	h.logger.Info("Starting TrazrExtension")

	// Initialize the ServiceResourceManager - this extension now owns it
	mgr, err := repository.NewServiceResourceManager(ctx, util.NewZapLoggerAdapter(h.logger))
	if err != nil {
		h.logger.Error("Failed to initialize ServiceResourceManager", zap.Error(err))
		return fmt.Errorf("failed to initialize service resource manager: %w", err)
	}
	db, err := mgr.GetDatabase()
	if err != nil || db == nil {
		_ = mgr.Shutdown()
		return fmt.Errorf("authentication database unavailable: %v", err)
	}
	if err := auth.EnsureAllAuthTables(db); err != nil {
		_ = mgr.Shutdown()
		return fmt.Errorf("failed to initialize authentication tables: %w", err)
	}
	// Reset delivery has one bounded queue owned by this extension. Missing or
	// invalid optional SMTP configuration leaves only that endpoint unavailable.
	if delivery, deliveryErr := auth.PasswordResetDeliveryFromEnv(); deliveryErr == nil {
		h.resetDispatcher, err = auth.NewPasswordResetDispatcher(delivery, pgstore.New(db))
		if err != nil {
			_ = mgr.Shutdown()
			return fmt.Errorf("failed to initialize password reset dispatcher: %w", err)
		}
	}

	h.mu.Lock()
	h.rsMgr = mgr
	h.mu.Unlock()

	// Signal that services are ready
	close(h.ready)

	// Optional: bootstrap demo/test user based on env vars (idempotent)
	if strings.EqualFold(strings.TrimSpace(os.Getenv("TRAZR_BOOTSTRAP_USER")), "true") {
		email := strings.TrimSpace(os.Getenv("TRAZR_BOOTSTRAP_USER_EMAIL"))
		password := os.Getenv("TRAZR_BOOTSTRAP_USER_PASSWORD")
		role := strings.TrimSpace(os.Getenv("TRAZR_BOOTSTRAP_USER_ROLE"))
		if email == "" || password == "" {
			h.logger.Warn("TRAZR_BOOTSTRAP_USER enabled but email or password missing; skipping")
		} else {
			var bootstrapErr error
			h.withDatabaseForBootstrap(func(db *sql.DB) {
				if err := auth.BootstrapUser(ctx, db, email, password, role); err != nil {
					bootstrapErr = err
					h.logger.Error("Bootstrap user failed", zap.Error(err))
				} else {
					h.logger.Info("Bootstrap user ensured", zap.String("email", email), zap.String("role", role))
				}
			})
			if bootstrapErr != nil {
				h.logger.Error("Failed to bootstrap user", zap.Error(bootstrapErr))
			}
		}
	}

	mux := http.NewServeMux()

	// Serve embedded OpenAPI spec (public) at canonical path
	mux.HandleFunc("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(openapiMergedYAML)
	})

	// Register health and version endpoints (public)
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		checks := map[string]string{}
		status := "ok"
		httpStatus := http.StatusOK

		// Check database connectivity
		db, err := h.rsMgr.GetDatabase()
		if err != nil || db == nil {
			checks["database"] = "unavailable"
			status = "degraded"
			httpStatus = http.StatusServiceUnavailable
		} else {
			pingCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := db.PingContext(pingCtx); err != nil {
				checks["database"] = "unreachable"
				status = "degraded"
				httpStatus = http.StatusServiceUnavailable
			} else {
				checks["database"] = "ok"
			}
		}

		w.WriteHeader(httpStatus)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": status,
			"checks": checks,
		})
	})
	mux.HandleFunc("/api/v1/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"version": "1.0.0"})
	})

	// Register schemas endpoint (JWT required)
	mux.Handle("/api/v1/schemas", auth.UserContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		internal.NewSchemasHandler(h.rsMgr)(w, r)
	})))

	// Register authentication endpoints using ServiceResourceManager (with request size limits)
	mux.HandleFunc("/api/v1/auth/register", internal.APIRequestLimiter(h.createAuthHandler(NewRegisterHandler)))
	mux.HandleFunc("/api/v1/auth/login", internal.APIRequestLimiter(h.createAuthHandler(NewLoginHandler)))
	mux.HandleFunc("/api/v1/auth/request-password-reset", internal.APIRequestLimiter(NewRequestPasswordResetHandlerWithDispatcher(h.resetDispatcher)))
	mux.HandleFunc("/api/v1/auth/reset-password", internal.APIRequestLimiter(h.createAuthHandler(NewResetPasswordHandler)))
	mux.Handle("/api/v1/auth/whoami", auth.UserContextMiddleware(internal.WithMaxBodySizeFunc(h.createAuthHandler(NewWhoAmIHandler), internal.MaxAPIRequestSize)))
	mux.Handle("/api/v1/auth/renew", auth.UserContextMiddleware(internal.WithMaxBodySizeFunc(h.createAuthHandler(NewRenewTokenHandler), internal.MaxAPIRequestSize)))
	mux.Handle("/api/v1/auth/logout", auth.UserContextMiddleware(internal.WithMaxBodySizeFunc(h.createAuthHandler(NewLogoutHandler), internal.MaxAPIRequestSize)))

	// API key management (JWT required)
	mux.Handle("/api/v1/auth/apikeys", auth.UserContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.withDatabase(w, func(db *sql.DB) {
			switch r.Method {
			case http.MethodGet:
				NewListAPIKeysHandler(db)(w, r)
			case http.MethodPost:
				NewCreateAPIKeyHandler(db)(w, r)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		})
	})))

	// API key revoke by id (JWT required)
	// Note: use a separate pattern with trailing slash to capture the id segment
	mux.Handle("/api/v1/auth/apikeys/", auth.UserContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			h.withDatabase(w, func(db *sql.DB) {
				NewRevokeAPIKeyHandler(db)(w, r)
			})
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})))

	// API key exchange (no JWT; uses X-API-Key)
	mux.HandleFunc("/api/v1/auth/apikeys/exchange", h.createAuthHandler(NewExchangeAPIKeyHandler))

	// Register admin endpoints using ServiceResourceManager (admin required, with request size limits)
	mux.HandleFunc("/api/v1/admin/users", internal.APIRequestLimiter(AdminOnlyMiddleware(h.createAuthHandler(NewListUsersHandler))))
	mux.HandleFunc("/api/v1/admin/user/delete", internal.APIRequestLimiter(AdminOnlyMiddleware(h.createAuthHandler(NewDeleteUserHandler))))
	mux.HandleFunc("/api/v1/admin/user/edit", internal.APIRequestLimiter(AdminOnlyMiddleware(h.createAuthHandler(NewEditUserHandler))))
	mux.HandleFunc("/api/v1/admin/user/reset-password", internal.APIRequestLimiter(AdminOnlyMiddleware(h.createAuthHandler(NewResetUserPasswordHandler))))
	mux.HandleFunc("/api/v1/admin/users/", internal.APIRequestLimiter(AdminOnlyMiddleware(h.createAuthHandler(NewDeleteUserByIDHandler))))
	mux.HandleFunc("/api/v1/admin/user/roles", internal.APIRequestLimiter(AdminOnlyMiddleware(h.createAuthHandler(NewSetUserRolesHandler))))
	mux.HandleFunc("/api/v1/admin/user/access", internal.APIRequestLimiter(AdminOnlyMiddleware(h.createAuthHandler(NewUserAccessHandler))))
	mux.HandleFunc("/api/v1/admin/user/schemas", internal.APIRequestLimiter(AdminOnlyMiddleware(h.createAuthHandler(NewSetUserSchemasHandler))))
	mux.HandleFunc("/api/v1/admin/user/revoke-sessions", internal.APIRequestLimiter(AdminOnlyMiddleware(h.createAuthHandler(NewRevokeUserSessionsHandler))))
	mux.HandleFunc("/api/v1/admin/user/disabled", internal.APIRequestLimiter(AdminOnlyMiddleware(h.createAuthHandler(NewSetUserDisabledHandler))))

	// Get ready-to-use services from ServiceResourceManager
	logWriterService, err := h.rsMgr.GetLogWriter()
	if err != nil {
		h.logger.Error("Failed to get log writer service", zap.Error(err))
		return err
	}

	traceWriterService, err := h.rsMgr.GetTraceWriter()
	if err != nil {
		h.logger.Error("Failed to get trace writer service", zap.Error(err))
		return err
	}

	metricWriterService, err := h.rsMgr.GetMetricWriter()
	if err != nil {
		h.logger.Error("Failed to get metric writer service", zap.Error(err))
		return err
	}

	profileWriterService, err := h.rsMgr.GetProfileWriter()
	if err != nil {
		h.logger.Error("Failed to get profile writer service", zap.Error(err))
		return err
	}

	// Get ready-to-use query services from ServiceResourceManager
	logQueryService, err := h.rsMgr.GetLogQuery()
	if err != nil {
		h.logger.Error("Failed to get log query service", zap.Error(err))
		return err
	}

	traceQueryService, err := h.rsMgr.GetTraceQuery()
	if err != nil {
		h.logger.Error("Failed to get trace query service", zap.Error(err))
		return err
	}

	metricQueryService, err := h.rsMgr.GetMetricQuery()
	if err != nil {
		h.logger.Error("Failed to get metric query service", zap.Error(err))
		return err
	}

	runWriterService, err := h.rsMgr.GetRunWriter()
	if err != nil {
		h.logger.Error("Failed to get run writer service", zap.Error(err))
		return err
	}

	runQueryService, err := h.rsMgr.GetRunQuery()
	if err != nil {
		h.logger.Error("Failed to get run query service", zap.Error(err))
		return err
	}

	profileQueryService, err := h.rsMgr.GetProfileQuery()
	if err != nil {
		h.logger.Error("Failed to get profile query service", zap.Error(err))
		return err
	}

	// Register OTEL data endpoints using dedicated route files
	internal.RegisterLogRoutes(mux, logWriterService, logQueryService)
	internal.RegisterTraceRoutes(mux, traceWriterService, traceQueryService)
	internal.RegisterMetricRoutes(mux, metricWriterService, metricQueryService)

	// Register runs and profiles endpoints using dedicated route files
	internal.RegisterRunRoutes(mux, runWriterService, runQueryService)
	internal.RegisterProfileRoutes(mux, profileWriterService, profileQueryService)

	// TODO: Re-enable vector store search endpoint after fixing interface issues
	// vectorStore, _ := h.rsMgr.GetVectorStore(schema)
	// vectorStoreSearchService := internal.NewVectorStoreSearchService(ProvideVectorStore(vectorStore))
	// mux.HandleFunc("/api/v1/vectorstore/search", auth.ScopedJWTAuthMiddleware(internal.NewVectorStoreSearchHandler(vectorStoreSearchService)))

	// Notifications enqueue (JWT required) via EventBus
	if eventBus := h.getEventBusOrLog("notifications"); eventBus != nil {
		mux.Handle("/api/v1/user/notifications", auth.UserContextMiddleware(http.HandlerFunc(NewNotificationsEnqueueHandler(eventBus))))
	}

	// Register OTEL configuration management endpoints (Admin required)
	configHandler := internal.NewConfigHandler(h.rsMgr) // Pass resource manager for config operations
	internal.RegisterConfigRoutes(mux, configHandler)

	// Register WebSocket streaming endpoints using the filtered handlers, with JWT auth
	mux.HandleFunc("/api/v1/stream/logs", h.wsSchemaAuthHandlerWrapper(func(w http.ResponseWriter, r *http.Request, conn *websocket.Conn) {
		// For now, use the default schema - we'll need to pass this from the configuration
		if eventBus := h.getEventBusOrClose(conn, "log streaming"); eventBus != nil {
			internal.StreamLogs(repository.NewLogStream(eventBus), logQueryService, conn, r)
		}
	}))
	mux.HandleFunc("/api/v1/stream/spans", h.wsSchemaAuthHandlerWrapper(func(w http.ResponseWriter, r *http.Request, conn *websocket.Conn) {
		if eventBus := h.getEventBusOrClose(conn, "trace streaming"); eventBus != nil {
			internal.StreamSpans(repository.NewTraceStream(eventBus), traceQueryService, conn, r)
		}
	}))
	// Metrics streams consolidated: dispatch on ?type=
	mux.HandleFunc("/api/v1/stream/metrics", h.wsSchemaAuthHandlerWrapper(func(w http.ResponseWriter, r *http.Request, conn *websocket.Conn) {
		if eventBus := h.getEventBusOrClose(conn, "metrics streaming"); eventBus != nil {
			internal.StreamMetrics(
				repository.NewGaugeMetricStream(eventBus),
				repository.NewSumMetricStream(eventBus),
				repository.NewHistogramMetricStream(eventBus),
				repository.NewSummaryMetricStream(eventBus),
				repository.NewExponentialHistogramMetricStream(eventBus),
				metricQueryService,
				conn,
				r,
			)
		}
	}))
	// Notifications stream (JWT required) via EventBus
	mux.HandleFunc("/api/v1/stream/user/notifications", h.wsScopedAuthHandlerWrapper(func(w http.ResponseWriter, r *http.Request, conn *websocket.Conn) {
		if eventBus := h.getEventBusOrClose(conn, "notifications streaming"); eventBus != nil {
			StreamNotificationsWS(eventBus, conn, r)
		}
	}))
	// Runs stream (JWT required)
	mux.HandleFunc("/api/v1/stream/runs", h.wsSchemaAuthHandlerWrapper(func(w http.ResponseWriter, r *http.Request, conn *websocket.Conn) {
		if eventBus := h.getEventBusOrClose(conn, "runs streaming"); eventBus != nil {
			internal.StreamRunsWSWithPreload(repository.NewEventBusRunStreamRepository(eventBus), runQueryService, conn, r)
		}
	}))

	// Register performance monitor stream (JWT required)
	mux.HandleFunc("/api/v1/stream/perfmonitor", h.wsAuthHandlerWrapper(func(w http.ResponseWriter, r *http.Request, conn *websocket.Conn) {
		internal.StreamPerfMonitorWS(conn, r)
	}))

	endpoint := h.cfg.Endpoint
	if endpoint == "" {
		endpoint = ":18888"
	}
	h.server = &http.Server{
		Addr: endpoint,
		Handler: withSecurityHeaders(withCORS(h.cfg.AllowedOrigins, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			withMachineScopes(mux).ServeHTTP(w, r.WithContext(auth.WithSessionDatabase(r.Context(), db)))
		}))),
		ReadTimeout:    15 * time.Second,
		WriteTimeout:   15 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	go func() {
		if err := h.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			h.logger.Error("HTTP server failed", zap.Error(err))
		}
	}()

	h.logger.Info("TrazrExtension started successfully")
	return nil
}

func (h *TrazrExtension) Shutdown(ctx context.Context) error {
	h.logger.Info("Shutting down TrazrExtension")
	if h.shutdownCancel != nil {
		h.shutdownCancel()
	}
	// Close all open WebSocket connections
	h.wsConns.Range(func(key, _ interface{}) bool {
		conn := key.(*websocket.Conn)
		conn.Close()
		return true
	})

	// Stop serving requests before private delivery workers and their database.
	var shutdownErr error
	if h.server != nil {
		shutdownErr = h.server.Shutdown(ctx)
		if shutdownErr != nil {
			_ = h.server.Close()
		}
	}
	if h.resetDispatcher != nil {
		// Token revocation must still finish if Collector's shutdown deadline has
		// expired. Delivery cancellation bounds cleanup to five seconds.
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		if err := h.resetDispatcher.Stop(cleanupCtx); err != nil {
			h.logger.Error("Password reset dispatcher shutdown failed")
			if shutdownErr == nil {
				shutdownErr = err
			}
		}
		cancel()
		h.resetDispatcher = nil
	}

	// Shutdown the ServiceResourceManager that this extension owns
	h.mu.Lock()
	if h.rsMgr != nil {
		if err := h.rsMgr.Shutdown(); err != nil {
			h.logger.Error("Failed to shutdown ServiceResourceManager", zap.Error(err))
		}
		h.rsMgr = nil
	}
	h.mu.Unlock()

	return shutdownErr
}

// Factory registration for the Trazr extension
func NewFactory() extension.Factory {
	return extension.NewFactory(
		component.MustNewType(TrazrExtensionType),
		createDefaultTrazrExtensionConfig, // returns *Config
		createTrazrExtension,
		component.StabilityLevelDevelopment,
	)
}

// createDefaultTrazrExtensionConfig returns a pointer to the correct config type
func createDefaultTrazrExtensionConfig() component.Config {
	return &Config{
		Endpoint:              ":18888",
		MaxConnections:        20,
		MaxIdleConnections:    10,
		ConnectionMaxLifetime: 30 * time.Minute,
	}
}

// createTrazrExtension is the factory method for the extension
func createTrazrExtension(
	ctx context.Context,
	set extension.Settings,
	cfg component.Config,
) (extension.Extension, error) {
	return newTrazrExtension(ctx, set, cfg)
}

// newTrazrExtension constructs the extension instance from config and settings.
func newTrazrExtension(
	_ context.Context,
	set extension.Settings,
	cfg component.Config,
) (extension.Extension, error) {
	c, ok := cfg.(*Config)
	if !ok {
		return nil, fmt.Errorf("invalid config type %T", cfg)
	}
	logger := set.TelemetrySettings.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	tracer := set.TelemetrySettings.TracerProvider
	var tr trace.Tracer
	if tracer != nil {
		tr = tracer.Tracer("github.com/medxops/trazr-otel/trazrextension")
	} else {
		tr = noop.NewTracerProvider().Tracer("github.com/medxops/trazr-otel/trazrextension")
	}
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	return &TrazrExtension{
		logger:         logger,
		tracer:         tr,
		cfg:            c,
		wsUpgrader:     newWSUpgrader(c.AllowedOrigins),
		shutdownCtx:    shutdownCtx,
		shutdownCancel: shutdownCancel,
		ready:          make(chan struct{}),
	}, nil
}

// getEventBusOrLog is a DRY helper that gets the event bus or logs an error
func (h *TrazrExtension) getEventBusOrLog(operation string) event.EventBusInterface {
	eventBus, err := h.rsMgr.GetEventBus()
	if err != nil {
		h.logger.Error("Failed to get event bus for "+operation, zap.Error(err))
		return nil
	}
	return eventBus
}

// getEventBusOrClose is a DRY helper that gets the event bus or closes the connection
func (h *TrazrExtension) getEventBusOrClose(conn *websocket.Conn, operation string) event.EventBusInterface {
	eventBus, err := h.rsMgr.GetEventBus()
	if err != nil {
		h.logger.Error("Failed to get event bus for "+operation, zap.Error(err))
		conn.Close()
		return nil
	}
	return eventBus
}

// createAuthHandler is a DRY helper that creates auth handlers with database from ServiceResourceManager
func (h *TrazrExtension) createAuthHandler(handlerFactory func(*sql.DB) http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.withDatabase(w, func(db *sql.DB) {
			handlerFactory(db)(w, r.WithContext(auth.WithSessionDatabase(r.Context(), db)))
		})
	}
}

// withDatabase is a DRY helper that handles database access pattern for HTTP handlers
func (h *TrazrExtension) withDatabase(w http.ResponseWriter, fn func(*sql.DB)) {
	db, err := h.rsMgr.GetDatabase()
	if err != nil {
		h.logger.Error("Failed to get database", zap.Error(err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if db == nil {
		h.logger.Error("Database is nil from ServiceResourceManager")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	fn(db)
}

// withDatabaseForBootstrap is a DRY helper that handles database access for bootstrap operations
func (h *TrazrExtension) withDatabaseForBootstrap(fn func(*sql.DB)) {
	db, err := h.rsMgr.GetDatabase()
	if err != nil {
		h.logger.Error("Failed to get database for bootstrap", zap.Error(err))
		return
	}
	if db == nil {
		h.logger.Error("Database is nil from ServiceResourceManager for bootstrap")
		return
	}
	fn(db)
}
