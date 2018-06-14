FROM alpine:3.6
WORKDIR /
# Now just add the binary
COPY goFidoGo /
ENTRYPOINT ["/goFidoGo"]
