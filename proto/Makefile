all:
	protoc -I=. --go_out=plugins=grpc:../client image_processor_service.proto
	protoc -I=. --go_out=plugins=grpc:../server image_processor_service.proto

.PHONY clean:
clean:
	rm -rf ../client/pb ../server/pb