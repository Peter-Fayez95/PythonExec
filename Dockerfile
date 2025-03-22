# Use a lightweight base image for the runtime
FROM alpine:latest
WORKDIR /app
COPY . .
RUN apk add --no-cache python3 go
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o myapi main.go
EXPOSE 8080
CMD ["./myapi"]