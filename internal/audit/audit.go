package audit

import (
"encoding/binary"
"fmt"
"os"
"sync"

"google.golang.org/protobuf/proto"

auditpb "arb/proto/gen/audit"
)

type Logger struct {
mu   sync.Mutex
file *os.File
}

func NewLogger(path string) (*Logger, error) {
f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
if err != nil {
return nil, fmt.Errorf("open audit log: %w", err)
}
return &Logger{file: f}, nil
}

func (l *Logger) Log(ev *auditpb.AuditEvent) error {
if l == nil {
return nil
}
l.mu.Lock()
defer l.mu.Unlock()
b, err := proto.Marshal(ev)
if err != nil {
return fmt.Errorf("marshal audit event: %w", err)
}
buf := make([]byte, binary.MaxVarintLen64)
n := binary.PutUvarint(buf, uint64(len(b)))
if _, err := l.file.Write(buf[:n]); err != nil {
return err
}
_, err = l.file.Write(b)
return err
}

func (l *Logger) Close() error {
if l == nil {
return nil
}
return l.file.Close()
}
