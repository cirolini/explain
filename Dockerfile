FROM golang:1.19-alpine
WORKDIR /usr/src/app
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN chmod +x script.sh && go build -o . cmd/explain/explain.go
CMD ./script.sh