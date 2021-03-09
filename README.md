#gRPC Cloud Detection

Exploration of uploading images via gRPC for facial detection in go.

### Requirements
- Go
- Protoc

### Running
```shell
cd proto && make
cd ..

cd server && make
./server

# new terminal tab
cd client && make
./client
```