FROM --platform=$BUILDPLATFORM golang:alpine AS build
WORKDIR /src
COPY go.mod go.sum .
RUN go mod download
COPY . .
ARG TARGETOS TARGETARCH
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/task .

FROM alpine:latest
RUN apk update && apk add bash && apk --no-cache add tzdata
COPY --from=build /out/task /bin
# Expose the port your application listens on
EXPOSE 8364

CMD ["/bin/task"]
