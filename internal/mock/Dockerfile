FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /mock-server ./internal/mock/cmd/

FROM alpine:3.20
RUN apk --no-cache add ca-certificates
COPY --from=build /mock-server /mock-server
EXPOSE 8080
ENTRYPOINT ["/mock-server"]
