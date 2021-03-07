package main

import (
	"bytes"
	"context"
	"gocv.io/x/gocv"
	"google.golang.org/grpc"
	pb "grpc-cloud-detection/client/v1/pb"
	"image"
	"io"
	"log"
	"time"
)

func CaptureImage(c pb.ImageProcessorClient) {
	camera, err := gocv.VideoCaptureDevice(0)
	if err != nil {
		log.Fatalf("failed to open camera: %v", err)
	}
	defer camera.Close()

	camera.Set(gocv.VideoCaptureFrameHeight, 64)

	window := gocv.NewWindow("Passed")
	defer window.Close()

	img := gocv.NewMat()
	defer img.Close()

	frames := 0
	start := time.Now()
	for {
		for i := 0; i < 100; i++ {
			ok := camera.Read(&img)
			if !ok {
				return
			}
			if img.Empty() {
				continue
			}

			//window.IMShow(img)

			image, err := img.ToImage()
			if err != nil {
				log.Printf("frame dropped: %v", err)
				continue
			}

			bounds := image.Bounds()
			x, y := bounds.Dx(), bounds.Dy()
			bytes := make([]byte, 0, x*y)
			for j := bounds.Min.Y; j < bounds.Max.Y; j++ {
				for i := bounds.Min.X; i < bounds.Max.X; i++ {
					r, g, b, a := image.At(i, j).RGBA()
					bytes = append(bytes, byte(b>>8))
					bytes = append(bytes, byte(g>>8))
					bytes = append(bytes, byte(r>>8))
					bytes = append(bytes, byte(a>>8))
				}
			}

			uploadedBytes := UploadImage(c, bytes, bounds)
			mat, err := gocv.NewMatFromBytes(y, x, gocv.MatTypeCV8UC4, uploadedBytes)
			if err != nil {
				continue
			}
			window.IMShow(mat)
			frames++;

			if window.WaitKey(1) >= 0 {
				break
			}
		}
		log.Printf("fps: %v", float64(frames) / time.Since(start).Seconds())
	}

}

func UploadImage(c pb.ImageProcessorClient, content []byte, bounds image.Rectangle) []byte{
	stream, err := c.DetectFaces(context.Background())
	if err != nil {
		log.Fatalf("error while calling Passthrough: %v", err)
	}

	waitc := make(chan bytes.Buffer)

	// send go routine
	go func() {

		boundaries := &pb.UploadImageRequest{
			Data: &pb.UploadImageRequest_Bounds{
				Bounds: &pb.ImageBounds{
					MinX: int64(bounds.Min.X),
					MinY: int64(bounds.Min.Y),
					MaxX: int64(bounds.Max.X),
					MaxY: int64(bounds.Max.Y),
				},
			},
		}

		err := stream.Send(boundaries)
		if err != nil {
			log.Printf("failed to upload bounds: %v", err)
			stream.CloseSend()
			return
		}

		size := 1024
		chunk := make([]byte, 1024)
		for len(content) >= size {
			chunk, content = content[:size], content[size:]
			err := stream.Send(&pb.UploadImageRequest{Data: &pb.UploadImageRequest_DataChunk{DataChunk: chunk}})
			if err != nil {
				log.Printf("failed to upload image: %v", err)
				stream.CloseSend()
				return
			}
		}
		if len(content) > 0 {
			chunk = content[:]
			err := stream.Send(&pb.UploadImageRequest{Data: &pb.UploadImageRequest_DataChunk{DataChunk: chunk}})
			if err != nil {
				log.Printf("failed to upload image: %v", err)
				stream.CloseSend()
				return
			}
		}
		stream.CloseSend()
	}()

	// recv go routine
	go func() {
		img := bytes.Buffer{}
		for {
			res, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Fatalf("error while reading upload image server stream: %v", err)
				break
			}

			_, err = img.Write(res.GetDataChunk())
		}

		//log.Printf("bytes received: %v", img.Len())

		waitc<- img
	}()

	img :=  <-waitc
	return img.Bytes()
}

func main() {
	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("could not connect: %v", err)
	}
	defer conn.Close()

	c := pb.NewImageProcessorClient(conn)

	CaptureImage(c)
}