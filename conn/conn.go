package conn

import (
	"crypto/tls"
	"fmt"
	dataConn "github.com/tianlin0/plat-lib/conn"
	"github.com/tianlin0/plat-lib/logs"
	"go.temporal.io/sdk/client"
	"net"
	"sync"
)

var (
	clientMapStore = sync.Map{}
)

func getClientCacheKey(conn *dataConn.Connect, certPath, keyPath string) string {
	return fmt.Sprintf("%s@%s:%s/%s/%s", conn.Username, conn.Host, conn.Port, certPath, keyPath)
}

func GetTemporalClient(conn *dataConn.Connect, certPath, keyPath string) (client.Client, error) {
	cacheKey := getClientCacheKey(conn, certPath, keyPath)
	if c, ok := clientMapStore.Load(cacheKey); ok {
		if clientTemp, ok := c.(client.Client); ok {
			return clientTemp, nil
		}
	}

	var cert tls.Certificate
	var hasCert = false
	if keyPath != "" && certPath != "" {
		var err error
		cert, err = tls.LoadX509KeyPair(certPath, keyPath)
		hasCert = true
		if err != nil {
			hasCert = false
			logs.DefaultLogger().Error("Unable to load cert and key pair.", err)
			return nil, err
		}
	}

	hostPort := net.JoinHostPort(conn.Host, conn.Port)
	dialOption := client.Options{
		HostPort:          hostPort,
		Namespace:         conn.Username,
		ConnectionOptions: client.ConnectionOptions{},
	}

	if hasCert {
		dialOption.ConnectionOptions.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	}

	temporalClient, err := client.Dial(dialOption)
	if err != nil {
		logs.DefaultLogger().Error("Unable to connect to Temporal Cloud.", err)
		return nil, err
	}

	clientMapStore.Store(cacheKey, temporalClient)

	return temporalClient, nil
}
