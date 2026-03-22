package fileproto

import (
	"context"
	"fmt"
	"io"
)

const transferChunkSize = 64 * 1024 // 64 KB

// TransferProgress is called periodically during a transfer.
type TransferProgress func(bytesTransferred int64, totalSize int64)

// Transfer streams a file from src to dst with progress reporting.
func Transfer(
	ctx context.Context,
	srcFS RemoteFS, srcPath string,
	dstFS RemoteFS, dstPath string,
	progress TransferProgress,
) (int64, error) {
	reader, size, err := srcFS.Read(ctx, srcPath)
	if err != nil {
		return 0, fmt.Errorf("read source: %w", err)
	}
	defer reader.Close()

	// Wrap reader with progress tracking and cancellation.
	pr := &progressReader{
		ctx:      ctx,
		reader:   reader,
		total:    size,
		progress: progress,
	}

	if err := dstFS.Write(ctx, dstPath, pr, size); err != nil {
		return pr.transferred, fmt.Errorf("write dest: %w", err)
	}
	return pr.transferred, nil
}

type progressReader struct {
	ctx         context.Context
	reader      io.Reader
	transferred int64
	total       int64
	progress    TransferProgress
}

func (pr *progressReader) Read(p []byte) (int, error) {
	// Check for cancellation before each read.
	if err := pr.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := pr.reader.Read(p)
	pr.transferred += int64(n)
	if pr.progress != nil && n > 0 {
		pr.progress(pr.transferred, pr.total)
	}
	return n, err
}
