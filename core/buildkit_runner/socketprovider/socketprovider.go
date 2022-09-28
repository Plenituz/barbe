package socketprovider

import (
	"context"
	"fmt"

	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/sshforward"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type Socket struct {
	Unix  string
	Npipe string
}

type SocketProvider struct {
}

func NewDockerSocketProvider() session.Attachable {
	return &SocketProvider{}
}

func (sp *SocketProvider) Register(server *grpc.Server) {
	sshforward.RegisterSSHServer(server, sp)
}

func (sp *SocketProvider) CheckAgent(ctx context.Context, req *sshforward.CheckAgentRequest) (*sshforward.CheckAgentResponse, error) {
	return &sshforward.CheckAgentResponse{}, nil
}

func (sp *SocketProvider) ForwardAgent(stream sshforward.SSH_ForwardAgentServer) error {
	id := sshforward.DefaultID
	opts, _ := metadata.FromIncomingContext(stream.Context()) // if no metadata continue with empty object
	if v, ok := opts[sshforward.KeySSHID]; ok && len(v) > 0 && v[0] != "" {
		id = v[0]
	}

	if id != "docker.sock" {
		return fmt.Errorf("invalid socket id %s", id)
	}

	socket := Socket{
		Unix:  "/var/run/docker.sock",
		Npipe: "//./pipe/docker_engine",
	}
	conn, err := dialSocket(socket)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", id, err)
	}
	defer conn.Close()

	return sshforward.Copy(context.TODO(), conn, stream, nil)
}
