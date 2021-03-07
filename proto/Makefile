all:
	protoc -I=. --go_out=plugins=grpc:../client process_image_service.proto
	protoc -I=. --go_out=plugins=grpc:../server process_image_service.proto