package main

import (
	"bytes"
	"errors"
	"gocv.io/x/gocv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"grpc-cloud-detection/server/v1/pb"
	"image"
	"image/color"
	"io"
	"log"
	"net"
)

type server struct {}

func (s *server) DetectFaces(stream pb.ImageProcessor_DetectFacesServer) error {
	req, err := stream.Recv()
	if err == io.EOF {
		return nil
	}
	if err != nil {
		log.Fatal("Error while reading client stream: ", err)
		return err
	}

	bounds := req.GetBounds()
	rectangle := image.Rectangle{
		Min: image.Point{
			X: int(bounds.GetMinX()),
			Y: int(bounds.GetMinY()),
		},
		Max: image.Point{
			X: int(bounds.GetMaxX()),
			Y: int(bounds.GetMaxY()),
		},
	}

	buf := bytes.Buffer{}

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal("Error while reading client stream: ", err)
			return err
		}

		buf.Write(req.GetDataChunk())
	}


	mat, err := gocv.NewMatFromBytes(rectangle.Dy(), rectangle.Dx(), gocv.MatTypeCV8UC4, buf.Bytes())

	classifier := gocv.NewCascadeClassifier()
	defer classifier.Close()

	if !classifier.Load("./data/haarcascade_frontalface_default.xml") {
		log.Printf("error reading cascade file")
		return errors.New("failed to read cascade file")
	}

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
		3)
	}

	img, err := mat.ToImage()
	if err != nil {
		return err
	}

	content := make([]byte, 0, rectangle.Dx() * rectangle.Dy())
	for j := rectangle.Min.Y; j < rectangle.Max.Y; j++ {
		for i := rectangle.Min.X; i < rectangle.Max.X; i++ {
			r, g, b, a := img.At(i, j).RGBA()
			content = append(content, byte(b>>8))
			content = append(content, byte(g>>8))
			content = append(content, byte(r>>8))
			content = append(content, byte(a>>8))
		}
	}

	size := 1024
	chunk := make([]byte, 1024)
	for len(content) >= size {
		chunk, content = content[:size], content[size:]
		err := stream.Send(&pb.UploadImageResponse{DataChunk: chunk})
		if err != nil {
			log.Printf("failed to upload image: %v", err)
			return err
		}
	}
	if len(content) > 0 {
		chunk = content[:]
		err := stream.Send(&pb.UploadImageResponse{
			DataChunk: chunk,
		})
		if err != nil {
			log.Printf("failed to upload image: %v", err)
			return err
		}
	}

	return nil
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
