# Build image
FROM golang:1.15-alpine AS build-env
WORKDIR /root/
COPY ./ ./
RUN CGO_ENABLED=0 GOOS=linux go build -a -o gym-tracker

# Runtime image
FROM chromedp/headless-shell:latest AS runtime-env
WORKDIR /root/
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get -y install tini ca-certificates
COPY --from=build-env /root/gym-tracker ./
ENTRYPOINT ["tini", "--", "/root/gym-tracker"]
