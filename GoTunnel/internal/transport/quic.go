package transport

import (
	"context"
	"crypto/tls"
	"errors"

	"github.com/quic-go/quic-go"
)

type QUICOptions struct {
	Addr               string
	ServerName         string
	CertFile           string
	KeyFile            string
	InsecureSkipVerify bool
}

func ListenQUIC(opts QUICOptions) (*quic.Listener, error) {
	if opts.CertFile == "" || opts.KeyFile == "" {
		return nil, errors.New("quic requires tls cert and key")
	}
	cert, err := tls.LoadX509KeyPair(opts.CertFile, opts.KeyFile)
	if err != nil {
		return nil, err
	}
	return quic.ListenAddr(opts.Addr, &tls.Config{
		MinVersion:   tls.VersionTLS13,
		NextProtos:   []string{"gotunnel-quic"},
		Certificates: []tls.Certificate{cert},
	}, &quic.Config{
		EnableDatagrams: true,
		KeepAlivePeriod: 20,
	})
}

func DialQUIC(ctx context.Context, opts QUICOptions) (*quic.Conn, error) {
	return quic.DialAddr(ctx, opts.Addr, &tls.Config{
		MinVersion:         tls.VersionTLS13,
		ServerName:         opts.ServerName,
		NextProtos:         []string{"gotunnel-quic"},
		InsecureSkipVerify: opts.InsecureSkipVerify,
	}, &quic.Config{
		EnableDatagrams: true,
		KeepAlivePeriod: 20,
	})
}
