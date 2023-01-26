# bufpipe
Buffered version of `io.Pipe`, it implements the same interface, but has internal buffer. This is useful if:
* writer produces data in bursts and reader should read at steady rate
* writer should not get blocked by slow reader

## Example

```
import "github.com/illarion/bufpipe"

...
maxSize = 1024 * 1024

r, w := bufpipe.Pipe(maxSize)
w.Write(...)
r.Read(...)

```

## Errors
`ErrBufferFull` - returned when reader is slower than writer and there is no place to write
`ErrClosedPipe` - returned if either reader or writer side is closed. 
