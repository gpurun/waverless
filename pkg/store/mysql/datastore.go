package mysql

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"golang.org/x/net/proxy"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"waverless/pkg/config"
)

// Dialect names supported by this package.
const (
	DialectMySQL    = "mysql"
	DialectPostgres = "postgres"
)

// Datastore wraps GORM DB and provides transaction support.
// Production uses PostgreSQL; MySQL helpers remain for legacy tests only.
type Datastore struct {
	db      *gorm.DB
	dialect string
}

// Dialect returns the underlying database dialect ("mysql" or "postgres").
func (ds *Datastore) Dialect() string {
	return ds.dialect
}

// IsPostgres reports whether the datastore is backed by PostgreSQL.
func (ds *Datastore) IsPostgres() bool {
	return ds.dialect == DialectPostgres
}

// newGormLogger builds the GORM logger used by all datastores.
func newGormLogger() logger.Interface {
	return logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             500 * time.Millisecond, // Slow SQL threshold
			LogLevel:                  logger.Warn,            // Log level (Warn in production, Info in dev)
			IgnoreRecordNotFoundError: true,                   // Ignore ErrRecordNotFound
			Colorful:                  true,                   // Enable color
		},
	)
}

// openWithDialector opens a GORM connection using the given dialector and
// configures the connection pool.
func openWithDialector(dialector gorm.Dialector, dialect string) (*Datastore, error) {
	db, err := gorm.Open(dialector, &gorm.Config{
		Logger:                 newGormLogger(),
		SkipDefaultTransaction: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Get underlying *sql.DB and configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get generic database object: %w", err)
	}

	// Connection pool settings
	sqlDB.SetMaxOpenConns(100)                 // Maximum open connections
	sqlDB.SetMaxIdleConns(10)                  // Maximum idle connections
	sqlDB.SetConnMaxLifetime(time.Hour)        // Connection max lifetime
	sqlDB.SetConnMaxIdleTime(10 * time.Minute) // Connection max idle time

	return &Datastore{db: db, dialect: dialect}, nil
}

// NewDatastore creates a new MySQL datastore
func NewDatastore(dsn string, proxyConfig *config.ProxyConfig) (*Datastore, error) {
	// Configure MySQL driver with custom dialer if proxy is enabled
	finalDSN := dsn

	if proxyConfig != nil && proxyConfig.Enabled {
		// Parse DSN to get MySQL config
		cfg, err := mysqldriver.ParseDSN(dsn)
		if err != nil {
			return nil, fmt.Errorf("failed to parse DSN: %w", err)
		}

		// Register custom dialer with proxy support
		dialerName := fmt.Sprintf("proxy_%s_%d", proxyConfig.Type, time.Now().UnixNano())
		mysqldriver.RegisterDialContext(dialerName, func(ctx context.Context, addr string) (net.Conn, error) {
			return dialWithProxy(ctx, addr, proxyConfig)
		})

		// Use the custom dialer
		cfg.Net = dialerName

		// Format DSN with custom dialer
		finalDSN = cfg.FormatDSN()

		log.Printf("MySQL proxy enabled: %s://%s:%d", proxyConfig.Type, proxyConfig.Host, proxyConfig.Port)
	}

	return openWithDialector(mysql.Open(finalDSN), DialectMySQL)
}

// NewPostgresDatastore creates a new PostgreSQL datastore.
// PostgreSQL connections use the native pgx transport and do not apply MySQL proxy configuration.
func NewPostgresDatastore(dsn string, proxyConfig *config.ProxyConfig) (*Datastore, error) {
	ds, err := openWithDialector(postgres.Open(dsn), DialectPostgres)
	if err != nil {
		return nil, err
	}
	sqlDB, err := ds.db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get PostgreSQL database handle: %w", err)
	}
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("failed to ping PostgreSQL database: %w", err)
	}
	return ds, nil
}

// dialWithProxy creates a connection through proxy
func dialWithProxy(ctx context.Context, addr string, proxyConfig *config.ProxyConfig) (net.Conn, error) {
	var baseDialer proxy.Dialer
	var err error

	proxyAddr := fmt.Sprintf("%s:%d", proxyConfig.Host, proxyConfig.Port)

	switch proxyConfig.Type {
	case "socks5":
		// Create SOCKS5 dialer
		baseDialer, err = proxy.SOCKS5("tcp", proxyAddr, nil, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("failed to create SOCKS5 proxy dialer: %w", err)
		}
	case "http", "https":
		// Create HTTP/HTTPS proxy dialer
		proxyURL, err := url.Parse(fmt.Sprintf("%s://%s", proxyConfig.Type, proxyAddr))
		if err != nil {
			return nil, fmt.Errorf("failed to parse proxy URL: %w", err)
		}

		// Use HTTP proxy dialer
		baseDialer = &httpProxyDialer{
			proxyURL: proxyURL,
			forward:  proxy.Direct,
		}
	default:
		return nil, fmt.Errorf("unsupported proxy type: %s (supported: http, https, socks5)", proxyConfig.Type)
	}

	return baseDialer.Dial("tcp", addr)
}

// httpProxyDialer implements proxy.Dialer for HTTP/HTTPS proxies
type httpProxyDialer struct {
	proxyURL *url.URL
	forward  proxy.Dialer
}

func (d *httpProxyDialer) Dial(network, addr string) (net.Conn, error) {
	// Connect to the proxy server
	conn, err := d.forward.Dial("tcp", d.proxyURL.Host)
	if err != nil {
		return nil, err
	}

	// For HTTPS proxy, establish TLS connection
	if d.proxyURL.Scheme == "https" {
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName: d.proxyURL.Hostname(),
		})
		if err := tlsConn.Handshake(); err != nil {
			conn.Close()
			return nil, err
		}
		conn = tlsConn
	}

	// Send HTTP CONNECT request
	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", addr, addr)
	if _, err := conn.Write([]byte(connectReq)); err != nil {
		conn.Close()
		return nil, err
	}

	// Read response
	response := make([]byte, 4096)
	n, err := conn.Read(response)
	if err != nil {
		conn.Close()
		return nil, err
	}

	// Check if connection was successful (HTTP 200)
	if n < 12 || string(response[9:12]) != "200" {
		conn.Close()
		return nil, fmt.Errorf("proxy connection failed: %s", string(response[:n]))
	}

	return conn, nil
}

// Close closes the database connection
func (ds *Datastore) Close() error {
	sqlDB, err := ds.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// Transaction support using context
type contextTxKey struct{}

// ExecTx executes a function within a transaction
// If the function returns an error, the transaction is rolled back
// Otherwise, the transaction is committed
func (ds *Datastore) ExecTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return ds.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ctx = context.WithValue(ctx, contextTxKey{}, tx)
		return fn(ctx)
	})
}

// DB returns the GORM DB instance for the current context
// If a transaction is active in the context, it returns the transaction DB
// Otherwise, it returns the main DB
func (ds *Datastore) DB(ctx context.Context) *gorm.DB {
	tx, ok := ctx.Value(contextTxKey{}).(*gorm.DB)
	if ok {
		return tx.WithContext(ctx)
	}
	return ds.db.WithContext(ctx)
}

// GetDB returns the underlying GORM DB instance (for direct access if needed)
func (ds *Datastore) GetDB() *gorm.DB {
	return ds.db
}
