/*******************************************************************************
 * Copyright (c) 2026 Genome Research Ltd.
 *
 * Author: Michael Woolnough <mw31@sanger.ac.uk>
 *
 * Permission is hereby granted, free of charge, to any person obtaining
 * a copy of this software and associated documentation files (the
 * "Software"), to deal in the Software without restriction, including
 * without limitation the rights to use, copy, modify, merge, publish,
 * distribute, sublicense, and/or sell copies of the Software, and to
 * permit persons to whom the Software is furnished to do so, subject to
 * the following conditions:
 *
 * The above copyright notice and this permission notice shall be included
 * in all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
 * EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
 * MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
 * IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
 * CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
 * TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
 * SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 ******************************************************************************/

package treegen

import (
	"context"
	"net"
	"strings"
	"time"
	"unsafe"

	"github.com/cyverse/go-irodsclient/irods/connection"
	"github.com/cyverse/go-irodsclient/irods/message"
	"github.com/cyverse/go-irodsclient/irods/types"
	"github.com/kuleuven/iron"
	"github.com/kuleuven/iron/api"
	"github.com/kuleuven/iron/msg"
)

type oldWrapper struct {
	*connection.IRODSConnection
}

func oldHandshake(env iron.Env) func(ctx context.Context) (iron.Conn, error) { //nolint:funlen
	return func(_ context.Context) (iron.Conn, error) {
		c, err := connection.NewIRODSConnection(
			&types.IRODSAccount{
				AuthenticationScheme:    types.AuthScheme(env.AuthScheme),
				ClientServerNegotiation: env.ClientServerNegotiation == "true",
				CSNegotiationPolicy:     types.CSNegotiationPolicyRequest(env.ClientServerNegotiationPolicy),
				Host:                    env.Host,
				Port:                    env.Port,
				ClientUser:              env.Username,
				ClientZone:              env.Zone,
				ProxyUser:               env.ProxyUsername,
				ProxyZone:               env.ProxyZone,
				Password:                env.Password,
				DefaultResource:         env.DefaultResource,
				PamTTL:                  int(env.GeneratedPasswordTimeout / time.Second),
				SSLConfiguration: &types.IRODSSSLConfig{
					CACertificateFile:       env.SSLCACertificateFile,
					EncryptionKeySize:       env.EncryptionKeySize,
					EncryptionAlgorithm:     env.EncryptionAlgorithm,
					EncryptionSaltSize:      env.EncryptionSaltSize,
					EncryptionNumHashRounds: env.EncryptionNumHashRounds,
					VerifyServer:            types.SSLVerifyServer(env.SSLVerifyServer),
					ServerName:              env.SSLServerName,
				},
			},
			&connection.IRODSConnectionConfig{
				ConnectTimeout:       time.Hour,
				OperationTimeout:     time.Hour,
				LongOperationTimeout: time.Hour,
				ApplicationName:      "backup-plans",
			},
		)
		if err != nil {
			return nil, err
		}

		if err := c.Connect(); err != nil {
			return nil, err
		}

		return &oldWrapper{c}, nil
	}
}

func (o *oldWrapper) Env() iron.Env { return iron.Env{} }

func (o *oldWrapper) Transport() net.Conn { return nil }

func (o *oldWrapper) ServerVersion() string {
	return strings.TrimPrefix(o.IRODSConnection.GetVersion().ReleaseVersion, "irods")
}

func (o *oldWrapper) ClientSignature() string {
	return o.GetClientSignature()
}

func (o *oldWrapper) NativePassword() string {
	return o.GetAccount().Password
}

func (o *oldWrapper) Request(_ context.Context, _ msg.APINumber, request, response any) error {
	var resp message.IRODSMessageQueryResponse

	o.Lock()
	defer o.Unlock()

	if err := o.IRODSConnection.Request( //nolint:forcetypeassert
		(*message.IRODSMessageQueryRequest)(unsafe.Pointer(request.(*msg.QueryRequest))), //nolint:errcheck
		&resp,
		nil,
		o.GetLongResponseOperationTimeout(),
	); err != nil {
		return err
	}

	*response.(*msg.QueryResponse) = *(*msg.QueryResponse)(unsafe.Pointer(&resp)) //nolint:errcheck,forcetypeassert

	return nil
}

func (o *oldWrapper) RequestWithBuffers(_ context.Context, _ msg.APINumber, _, _ any, _, _ []byte) error {
	return nil
}

func (o *oldWrapper) API() *api.API {
	account := o.GetAccount()

	return &api.API{
		Username:        account.ClientUser,
		Zone:            account.ClientZone,
		Connect:         func(context.Context) (api.Conn, error) { return o, nil },
		DefaultResource: account.DefaultResource,
	}
}

func (o *oldWrapper) Close() error {
	return o.Disconnect()
}
func (o *oldWrapper) RegisterCloseHandler(_ func() error) context.CancelFunc {
	return nil
}

func (o *oldWrapper) ConnectedAt() time.Time {
	return o.GetCreationTime()
}

func (o *oldWrapper) TransportErrors() int {
	return 0
}

func (o *oldWrapper) SQLErrors() int {
	return 0
}
