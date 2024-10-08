# zero-downtime-upgrade

This is a simple demo of zero-downtime upgrade implemented using `socket takeover` technology. The `fdtrans` directory is copied from [fd repo writen by ftrvxmtrx
](https://github.com/ftrvxmtrx/fd/tree/master), which impletents function that transfers file descriptors between local processes.

# how to run

## build binary executable files

```bash
go build -gcflags="all=-l -N" -output server.bin ./server
go build -gcflags="all=-l -N" -output client.bin ./client
```

## Run server

```bash
./server.bin
./client.bin
./server.bin
```

## Output

1. on server console one

```bash
./server.bin
2024/09/08 16:45:20 start as server side
recv:  hello server: 0
recv:  hello server: 1
recv:  hello server: 2
recv:  hello server: 3
recv:  hello server: 4
recv:  hello server: 5
recv:  hello server: 6
2024/09/08 16:46:14 conn closed, as server side
```

2. on server console two

```bash
❯ ./server.bin
2024/09/08 16:46:14 start as client side
recv:  hello server: 8
recv:  hello server: 9
recv:  hello server: 10
recv:  hello server: 11
recv:  hello server: 12
recv:  hello server: 13
recv:  hello server: 14
recv:  hello server: 15
recv:  hello server: 16
recv:  hello server: 17
```

3. on client console

```bash
❯ ./client.bin
client recv:  hello client: hello server: 0
client recv:  hello client: hello server: 1
client recv:  hello client: hello server: 2
client recv:  hello client: hello server: 3
client recv:  hello client: hello server: 4
client recv:  hello client: hello server: 5
client recv:  hello client: hello server: 6
2024/09/08 16:46:23 client read err: read unix @->/tmp/socket_takeover.sock: i/o timeout
client recv:  hello client, existing conn: hello server: 8
client recv:  hello client, existing conn: hello server: 9
client recv:  hello client, existing conn: hello server: 10
client recv:  hello client, existing conn: hello server: 11
client recv:  hello client, existing conn: hello server: 12
client recv:  hello client, existing conn: hello server: 13
client recv:  hello client, existing conn: hello server: 14
client recv:  hello client, existing conn: hello server: 15
client recv:  hello client, existing conn: hello server: 16
client recv:  hello client, existing conn: hello server: 17
2024/09/08 16:46:43 client read err: EOF
2024/09/08 16:46:43 write unix @->/tmp/socket_takeover.sock: write: broken pipe
```

## Note

This downtime during upgrading is `NOT EXACTLY` zero, but it's close to zero. As we can see from the client output, message 7 is unavailable because old server didn't finish sending it before fd closed, even though the new server has already taken control of the listened fd by socket takeover. This is the tradeoff between tolerance and complexity.
