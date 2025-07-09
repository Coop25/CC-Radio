package chunker

import "time"

// PrepareChunks splits data into fixed‐size slices given duration & interval.
func PrepareChunks(data []byte, total time.Duration, interval time.Duration) [][]byte {
    totalBytes := len(data)
    bytesPerSecond := float64(totalBytes) / total.Seconds()
    size := int(bytesPerSecond * interval.Seconds())
    if size <= 0 {
        panic("chunk size computed ≤ 0")
    }
    var slices [][]byte
    for off := 0; off < totalBytes; off += size {
        end := off + size
        if end > totalBytes {
            end = totalBytes
        }
        slices = append(slices, data[off:end])
    }
    return slices
}