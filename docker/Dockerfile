FROM --platform=$BUILDPLATFORM golang:alpine AS build
RUN apk add upx
WORKDIR $GOPATH/src/kproximate
COPY . .
ARG TARGETARCH
RUN cd kproximate && GOOS=linux GOARCH=$TARGETARCH go build -v -o /go/bin/kproximate
RUN upx /go/bin/kproximate

# final stage
FROM alpine

RUN adduser \    
    --disabled-password \    
    --gecos "" \    
    --home "/nonexistent" \    
    --shell "/sbin/nologin" \    
    --no-create-home \    
    --uid "1001" \    
    "kproximate"

COPY --from=build /go/bin/kproximate .
USER kproximate:kproximate
ENTRYPOINT ./kproximate