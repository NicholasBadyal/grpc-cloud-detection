package main

import (
	"context"
	"fmt"
	"gocv.io/x/gocv"
	"google.golang.org/grpc"
	"grpc-cloud-detection/client/v1/pb"
	"io"
	"log"
	"time"
)

func DetectFromCamera(c pb.ImageProcessorClient) error {
	// setup visual components
	camera, err := gocv.VideoCaptureDevice(0)
	if err != nil {
		log.Fatalf("failed to open camera: %v", err)
	}
	defer camera.Close()

	window := gocv.NewWindow("Passed")
	defer window.Close()

	img := gocv.NewMat()
	defer img.Close()

	// open stream to RPC
	stream, err := c.DetectFaces(context.Background())
	if err != nil {
		return fmt.Errorf("error while calling DetectFaces RPC: %v", err)
	}

	// channels for mats
	matsToSend := make(chan gocv.Mat, 20)
	matsToDisplay := make(chan gocv.Mat, 20)

	// goroutine receives response from server and creates mats to display
	go func() {
		for {
			res, err := stream.Recv()
			if err == io.EOF {
				log.Print("no more responses")
				break
			}
			if err != nil {
				log.Printf("failed to receive stream response: %v", err)
				break
			}

			newMat, err := gocv.NewMatFromBytes(int(res.GetRows()), int(res.GetCols()), gocv.MatType(res.EltType), res.GetMatData())
			if err != nil {
				log.Printf("failed to create mat from bytes: %v", err)
				continue
			}

			matsToDisplay <- newMat
		}
	}()

	// goroutine sends request to server
	go func() {
		for {
			img := <-matsToSend

			data, err := img.DataPtrUint8()

			if err != nil {
				log.Printf("failed to collect mat data: %v", err)
				break
			}

			mat := &pb.UploadMatRequest{
				Rows:    int32(img.Rows()),
				Cols:    int32(img.Cols()),
				EltType: int32(img.Type()),
				MatData: data,
			}

			err = stream.Send(mat)
			if err != nil {
				log.Printf("failed to send request: %v - %v", err, stream.RecvMsg(nil))
				break
			}
		}
		_ = stream.CloseSend()
	}()

	// displays mats and controls window
	frames := 0
	start := time.Now()
	for {
		ok := camera.Read(&img)
		if !ok {
			return fmt.Errorf("failed to read image from camera: %v", err)
		}
		if img.Empty() {
			continue
		}

		matsToSend <- img

		mat := <-matsToDisplay
		frames++

		window.IMShow(mat)

		if window.WaitKey(1) >= 0 {
			break
		}

		log.Printf("fps: %.4v\ttotal frames: %v", float64(frames)/time.Since(start).Seconds(), frames)
	}

	return nil

}

func main() {
	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("could not connect: %v", err)
	}

	c := pb.NewImageProcessorClient(conn)

	if err = DetectFromCamera(c); err != nil {
		log.Fatalf("failed to DetectFromCamera: %v", err)
	}

	if err = conn.Close(); err != nil {
		log.Fatalf("failed to close connection to server: %v", err)
	}

	log.Printf("Client closed")
}
