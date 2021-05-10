package main

import (
	"context"
	"grpc-chat-2/protos/protogen"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc"
	glog "google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/keepalive"
)

var grpcLog glog.LoggerV2

func init() {
	grpcLog = glog.NewLoggerV2(os.Stdout, os.Stdout, os.Stdout)
}

func main() {
	var connections []*Connection

	server := &Server{connections, protogen.UnimplementedBroadcastServer{}}

	grpcServer := grpc.NewServer(grpc.KeepaliveParams(keepalive.ServerParameters{
		MaxConnectionIdle: 5 * time.Minute,
	}))
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("Error creating the server: %v", err)
	}

	grpcLog.Info("Starting server at port 8080")

	protogen.RegisterBroadcastServer(grpcServer, server)
	grpcServer.Serve(listener)
}

type Connection struct {
	stream protogen.Broadcast_CreateStreamServer
	id     string
	active bool
	error  chan error
}

type Server struct {
	Connection []*Connection
	protogen.UnimplementedBroadcastServer
}

func (s *Server) CreateStream(pconn *protogen.Connect, stream protogen.Broadcast_CreateStreamServer) error {
	conn := &Connection{
		stream: stream,
		id:     pconn.User.Id,
		active: true,
		error:  make(chan error),
	}

	s.Connection = append(s.Connection, conn)

	return <-conn.error
}

func (s *Server) BroadcastMessage(ctx context.Context, msg *protogen.Message) (*protogen.Empty, error) {
	wait := sync.WaitGroup{}
	done := make(chan int)

	for _, conn := range s.Connection {
		wait.Add(1)

		go func(msg *protogen.Message, conn *Connection) {
			defer wait.Done()

			if conn.active {
				err := conn.stream.Send(msg)
				grpcLog.Info("Sending message to: ", conn.stream)

				if err != nil {
					grpcLog.Errorf("Error with Stream: %s - Error: %v", conn.stream, err)
					conn.active = false
					conn.error <- err
				}
			}
		}(msg, conn)
	}

	go func() {
		wait.Wait()
		close(done)
	}()

	<-done
	return &protogen.Empty{}, nil
}
