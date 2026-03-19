# Stage 1:
# Use base Alpine image to prepare our binary, label it 'app'
FROM golang:1.26.1-alpine AS app
# Add golangdocker user and group so that the Docker process in Scratch doesn't run as root
RUN addgroup -S go \
    && adduser -S -u 10000 -g go go
# Change to the correct directory to hold our application source code
WORKDIR /go/src/app
# Copy all the files from the base of our repository to the current directory defined above
COPY go.mod .
COPY go.sum .
COPY main.go .
# Compile the application to a single statically-linked binary file
RUN CGO_ENABLED=0 go install -ldflags '-extldflags "-static"' -tags timetzdata

# Stage 2:
# Use the Docker Scratch image to copy our previous stage into
FROM scratch
# Grab necessary certificates as Scratch has none
COPY --from=app /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
# Copy our binary to the root of the Scratch image (note: --from=app, the name we gave our first stage)
COPY --from=app /go/bin/power-api /power-api
# Copy the user that we created in the first stage so that we don't run the process as root
COPY --from=app /etc/passwd /etc/passwd
# Change to the non-root user
USER go
# Run our app by directly executing the binary
ENTRYPOINT ["/power-api"]
