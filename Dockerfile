FROM --platform=$BUILDPLATFORM golang:alpine AS build
WORKDIR /src
COPY go.mod go.sum .
RUN go mod download
COPY . .
ARG TARGETOS TARGETARCH
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/gophercal .

FROM alpine:latest
RUN apk update && apk add bash && apk --no-cache add tzdata
COPY --from=build /out/gophercal /bin
# Expose the port your application listens on
EXPOSE 8364

CMD ["/bin/gophercal"]