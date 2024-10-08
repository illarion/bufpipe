# bufpipe
Simple golang implementation of the buffered io.Pipe replacement

## Usage
```go
package main

import (
    "fmt"
    "io"
    "os"
    "time"

    "github.com/illarion/bufpipe/v3"
)

func main() {
    // Create a new buffered pipe
    reader, writer := bufpipe.Pipe(bufpipe.Options{
        MaxSize: 1024,
        BlockWritesUntilFirstReadTimeout: time.Second * 5,
		BlockWritesOnFullBufferTimeout: time.Second * 5,
    })

    // Start a goroutine that writes to the pipe
    go func() {
        defer writer.Close()
        for i := 0; i < 10; i++ {
            fmt.Fprintf(writer, "Hello, world! %d\n", i)
            time.Sleep(time.Second)
        }
    }()

    // Read from the pipe
    io.Copy(os.Stdout, reader)
}
```

## Options

- `MaxSize` - the maximum size of the buffer in bytes. Default is 0 - no limit.
- `BlockWritesUntilFirstReadTimeout` - if set, the writer will block until the first read from the reader or the timeout is reached. Default is 0 - no blocking, write immediately.
- `BlockWritesOnFullBufferTimeout` - if set, the writer will block until the buffer is not full or the timeout is reached. Default is 0 - immediately return `ErrBufferFull` if the buffer is full.

## Errors
`ErrBufferFull` - returned when reader is slower than writer and there is no place to write
`ErrClosedPipe` - returned if either reader or writer side is closed.
`ErrFirstReadTimeout` - returned if the `BlockWritesUntilFirstReadTimeout` is reached.


## License

See the [LICENSE](LICENSE) file for license rights and limitations (MIT).


