package main

import (
	"context"
	"gocv.io/x/gocv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"grpc-cloud-detection/server/v1/pb"
	"image/color"
	"io"
	"log"
	"net"
)

type server struct{}

// logs err
func logErr(err error) error{
	if err != nil {
		log.Print(err)
	}
	return err
}

// checks if request has been canceled or has passed deadline
func contextError(ctx context.Context) error {
	switch ctx.Err() {
	case context.Canceled:
		return logErr(status.Error(codes.Canceled, "request was canceled"))
	case context.DeadlineExceeded:
		return logErr(status.Error(codes.DeadlineExceeded, "deadline was exceeded"))
	default:
		return nil
	}
}

func (s *server) DetectFaces(stream pb.ImageProcessor_DetectFacesServer) error {
	classifier := gocv.NewCascadeClassifier()
	defer classifier.Close()

	if !classifier.Load("./data/haarcascade_frontalface_default.xml") {
		return logErr(status.Error(codes.Internal, "failed to open cascade"))
	}

	frame := 0

	// channels for passing mats between goroutines
	mats := make(chan gocv.Mat, 20)
	detected := make(chan gocv.Mat, 20)
	// channel for error checking
	errLock := make(chan error)

	// goroutine receives mats from client
	go func() {
		for {
			err := contextError(stream.Context())
			if err != nil {
				 errLock<- err
				 break
			}

			req, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				errLock<- logErr(status.Errorf(codes.Unknown, "cannot receive stream request: %v", err))
				break
			}

			log.Printf("Detecting faces on Mat %v",frame)
			frame++

			mat, err := gocv.NewMatFromBytes(
				int(req.GetRows()),
				int(req.GetCols()),
				gocv.MatType(req.GetEltType()),
				req.GetMatData(),
			)
			if err != nil {
				errLock<- logErr(status.Errorf(codes.Internal, "cannot create Mat from bytes: %v", err))
				break
			}

			mats<- mat
		}
	}()

	// goroutine applies classifier
	go func() {
		for {
			mat := <-mats
			rects := classifier.DetectMultiScale(mat)
			for _, r := range rects {
				gocv.Rectangle(
					&mat,
					r,
					color.RGBA{
						R: 200,
						G: 200,
						B: 255,
						A: 1,
					},
					3,
				)
			}
			detected<- mat
		}
	}()

	// goroutine sends mats to client
	go func() {
		for {
			err := contextError(stream.Context())
			if err != nil {
				errLock<- err
			}

			mat := <-detected
			data, err := mat.DataPtrUint8()
			if err != nil {
				errLock<- logErr(status.Errorf(codes.Internal, "cannot use data from mat: %v", err))
				break
			}
			detected := &pb.UploadMatResponse{
				Rows:    int32(mat.Rows()),
				Cols:    int32(mat.Cols()),
				EltType: int32(mat.Type()),
				MatData: data,
			}

			err = stream.Send(detected)
			if err != nil {
				errLock<- logErr(status.Errorf(codes.Unknown, "cannot send stream response: %v", err))
				break
			}
		}
	}()

	for {
		err := contextError(stream.Context())
		if err != nil {
			return err
		}

		// non-blocking check for errors in goroutines
		select {
		case err, ok := <-errLock:
			if ok {
				return err
			} else {
				return logErr(status.Errorf(codes.Internal, "channel close unexpectedly"))
			}
		default:
			continue
		}
	}
}

func main() {
	lis, err := net.Listen("tcp", "0.0.0.0:50051")
	if err != nil {
		log.Fatal("Failed to listen: ", err)
	}
	defer lis.Close()

	grpcServer := grpc.NewServer()
	pb.RegisterImageProcessorServer(grpcServer, &server{})

	reflection.Register(grpcServer)

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatal("failed to serve: ", err)
	}
}
