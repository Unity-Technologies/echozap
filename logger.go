package echozap

import (
	"fmt"
	"time"

	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type (
	Skipper func(c *echo.Context) bool

	// ZapLoggerConfig defines the config for ZapLogger middleware
	ZapLoggerConfig struct {
		// Skipper defines a function to skip middleware
		Skipper Skipper
		// Whether to include the request method and URI in the log message field
		// Makes it easier to visualize the logs in systems that expand only the log message by default(e.g. Stackdriver)
		IncludeRequestLogMessage bool
	}
)

var (
	// DefaultZapLoggerConfig is the default ZapLogger middleware config.
	DefaultZapLoggerConfig = ZapLoggerConfig{
		Skipper:                  DefaultSkipper,
		IncludeRequestLogMessage: false,
	}
)

// DefaultSkipper returns false which processes the middleware
func DefaultSkipper(*echo.Context) bool {
	return false
}

// ZapLogger is a middleware and zap to provide an "access log" like logging for each request.
func ZapLogger(log *zap.Logger) echo.MiddlewareFunc {
	return ZapLoggerWithConfig(log, DefaultZapLoggerConfig)
}

// ZapLoggerWithConfig is a middleware (with configuration) and zap to provide an "access log" like logging for each request.
func ZapLoggerWithConfig(log *zap.Logger, config ZapLoggerConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		// Defaults
		if config.Skipper == nil {
			config.Skipper = DefaultZapLoggerConfig.Skipper
		}

		return func(c *echo.Context) error {
			if config.Skipper(c) {
				return next(c)
			}

			start := time.Now()

			err := next(c)
			if err != nil {
				c.Echo().HTTPErrorHandler(c, err)
			}

			req := c.Request()

			// echo v5 returns the bare http.ResponseWriter from Response(); the recorded status and
			// size live on the underlying *echo.Response, recovered via UnwrapResponse.
			var status int
			var size int64
			if res, uErr := echo.UnwrapResponse(c.Response()); uErr == nil {
				status = res.Status
				size = res.Size
			}

			requestLogField := fmt.Sprintf("%s %s", req.Method, req.RequestURI)

			fields := []zapcore.Field{
				zap.String("remote_ip", c.RealIP()),
				zap.String("latency", time.Since(start).String()),
				zap.String("host", req.Host),
				zap.String("request", requestLogField),
				zap.Int("status", status),
				zap.Int64("size", size),
				zap.String("user_agent", req.UserAgent()),
			}

			id := req.Header.Get(echo.HeaderXRequestID)
			if id == "" {
				id = c.Response().Header().Get(echo.HeaderXRequestID)
			}
			fields = append(fields, zap.String("request_id", id))

			var requestLogMessage string

			if config.IncludeRequestLogMessage {
				requestLogMessage = ": " + requestLogField
			}

			n := status
			switch {
			case n >= 500:
				log.With(zap.Error(err)).Error("Server error"+requestLogMessage, fields...)
			case n >= 400:
				log.With(zap.Error(err)).Warn("Client error"+requestLogMessage, fields...)
			case n >= 300:
				log.Info("Redirection"+requestLogMessage, fields...)
			default:
				log.Info("Success"+requestLogMessage, fields...)
			}

			return nil
		}
	}
}
