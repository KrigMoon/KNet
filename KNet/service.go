package KNet

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/KrigMoon/KJournal"
	"github.com/KrigMoon/KNet/KNet/Commands"
	"github.com/KrigMoon/KNet/KNetCore"
	"github.com/KrigMoon/KNet/Proto"
	knetpb "github.com/KrigMoon/KNet/Proto/gen"
)

//region Metrics

type ReaderMetrics struct {
	ID       string
	Len, Cap int
}

type CursorMetrics struct {
	Cursor struct {
		ID, Executer   string
		LenLoc, LenExt int
	}
	Len int
	Cap int
}

type KNetMetrics struct {
	Status       knetpb.Status
	Cursor       CursorMetrics
	Responses    struct{ Len, Cap int }
	Core         KNetCore.KNetCoreMetrics
	ReadersCount int
	Readers      []ReaderMetrics
}

func (kn *KNet) GetMetrics() KNetMetrics {
	cursor := kn.GetCursor()

	cm := CursorMetrics{
		Len: len(kn.cursorQueue),
		Cap: cap(kn.cursorQueue),
	}
	cm.Cursor.ID = cursor.ID
	cm.Cursor.LenLoc = len(cursor.LocalProgram)
	cm.Cursor.LenExt = len(cursor.ExternalProgram)

	switch cursor.Executer {
	case Commands.ExecuterLocal:
		cm.Cursor.Executer = "LOCAL"
	case Commands.ExecuterExternal:
		cm.Cursor.Executer = "EXTERNAL"
	case Commands.ExecuterBoth:
		cm.Cursor.Executer = "BOTH"
	default:
		cm.Cursor.Executer = "UNKNOWN"
	}

	m := KNetMetrics{
		Status: kn.status,
		Cursor: cm,
		Responses: struct{ Len, Cap int }{
			Len: len(kn.responses),
			Cap: cap(kn.responses),
		},
		Core:         kn.knc.GetMetrics(),
		ReadersCount: int(kn.readersCount.Load()),
	}

	kn.readers.Range(func(key, v any) bool {
		ch := v.(ReaderPackages)
		m.Readers = append(m.Readers, ReaderMetrics{
			ID:  key.(string),
			Len: len(ch.Chan),
			Cap: cap(ch.Chan),
		})
		return true
	})

	return m
}

//endregion

//region Service methods

func (kn *KNet) GetPeerList() []KNetCore.PeerInfo {
	return kn.knc.GetPeerList()
}

func (kn *KNet) GetJournal() *KJournal.KJournal {
	return kn.settings.Journal
}

func (kn *KNet) GetLocalCommands() map[string]Commands.CommandContainer {
	return kn.localCommandsMap
}

func (kn *KNet) GetInterface() Commands.Interface {
	return Commands.Interface{
		KNet:     kn,
		KNetCore: &kn.knc,
	}
}

//endregion

//region FileReader

// desc: Assemble chunks into a file, verify hashes, and save
func (kn *KNet) assembleAndSaveFile(storage string, chunks []Commands.FileChunk) error {
	if len(chunks) == 0 {
		return fmt.Errorf("no chunks")
	}

	meta := chunks[0].Metadata

	//* Reorder by ChunkID (duplicates leave holes — caught by hash checks below)
	ordered := make([]Commands.FileChunk, len(chunks))
	for _, c := range chunks {
		if c.Metadata.ChunkID < 0 || c.Metadata.ChunkID >= len(ordered) {
			return fmt.Errorf("invalid ChunkID: %d", c.Metadata.ChunkID)
		}
		ordered[c.Metadata.ChunkID] = c
	}

	//* Verify chunk hashes and assemble
	fileHasher := sha256.New()
	content := make([]byte, 0)
	for _, c := range ordered {
		sum := sha256.Sum256(c.Chunk)
		if hex.EncodeToString(sum[:]) != c.Metadata.ChunkHash {
			return fmt.Errorf("chunk %d hash mismatch", c.Metadata.ChunkID)
		}
		content = append(content, c.Chunk...)
		fileHasher.Write(c.Chunk)
	}

	//* Verify file hash
	if hex.EncodeToString(fileHasher.Sum(nil)) != meta.FileHash {
		return fmt.Errorf("file hash mismatch")
	}

	//* Save via .part -> rename
	name := filepath.Base(meta.FileName)
	finalPath := filepath.Join(storage, name)
	partPath := finalPath + ".part"

	if err := os.WriteFile(partPath, content, 0o644); err != nil {
		_ = os.Remove(partPath)
		return fmt.Errorf("write %q: %w", partPath, err)
	}
	if err := os.Rename(partPath, finalPath); err != nil {
		_ = os.Remove(partPath)
		return fmt.Errorf("rename %q: %w", finalPath, err)
	}

	return nil
}

type fileSession struct {
	Chan       chan Commands.FileChunk
	LastChange time.Time
}

const (
	fileCollectorTick    = 30 * time.Second
	fileCollectorTimeout = 60 * time.Second
)

// desc: Run the file collector
func (kn *KNet) CreateFileCollector() error {
	storage, err := Commands.PrepareFileStorage(kn.settings.FileStorageDir)
	if err != nil {
		return fmt.Errorf("PrepareFileStorage: %s", err)
	}

	//* Create memory
	memory := map[string]*fileSession{}

	//* Register the reader
	readerID := "FileReader"
	readerID = kn.AddReaderPackages(&readerID, 256, func(p *knetpb.Package) bool {
		return p.DataType == knetpb.DataType_BYTES && p.Type == knetpb.PackageType_FILE
	})

	//* Launching the file collector
	kn.processes.Add(1)
	go func() {
		defer kn.processes.Done()
		defer kn.DelReaderPackages(readerID)

		ticker := time.NewTicker(fileCollectorTick)
		defer ticker.Stop()

		for {
			select {
			case <-kn.ctx.Done():
				return
			case <-ticker.C:
				now := time.Now()
				for name, s := range memory {
					if now.Sub(s.LastChange) > fileCollectorTimeout {
						kn.settings.Journal.Write(Proto.NewRecord(
							"KNet",
							knetpb.RecordLevel_WARNING,
							knetpb.RecordCode_DATA,
							fmt.Sprintf("File session %q timed out, dropped", name),
						))
						delete(memory, name)
					}
				}
				continue
			default:
			}

			pkg := kn.Package(readerID)
			if pkg == nil {
				time.Sleep(50 * time.Millisecond)
				continue
			}

			//* Translate pkg to chunk
			chunk, err := Commands.PackageToChunk(pkg)
			if err != nil {
				kn.settings.Journal.Write(Proto.NewRecord(
					"KNet",
					knetpb.RecordLevel_WARNING,
					knetpb.RecordCode_DATA,
					fmt.Sprintf("PackageToChunk: %s", err),
				))
				continue
			}
			fileName := chunk.Metadata.FileName

			s, find := memory[fileName]
			if !find {
				s = &fileSession{
					Chan:       make(chan Commands.FileChunk, chunk.Metadata.ChunkCount),
					LastChange: time.Now(),
				}
				memory[fileName] = s
			}

			select {
			case s.Chan <- chunk:
				s.LastChange = time.Now()
			default:
				kn.settings.Journal.Write(Proto.NewRecord(
					"KNet",
					knetpb.RecordLevel_WARNING,
					knetpb.RecordCode_DATA,
					fmt.Sprintf("Chunk overflow for %q: declared %d chunks, session aborted",
						fileName, chunk.Metadata.ChunkCount),
				))
				delete(memory, fileName)
				continue
			}

			//* All chunks received — assemble, verify, save
			if len(s.Chan) == cap(s.Chan) {
				chunks := make([]Commands.FileChunk, 0, cap(s.Chan))
				for i := 0; i < cap(s.Chan); i++ {
					chunks = append(chunks, <-s.Chan)
				}
				delete(memory, fileName)

				if err := kn.assembleAndSaveFile(storage, chunks); err != nil {
					kn.settings.Journal.Write(Proto.NewRecord(
						"KNet",
						knetpb.RecordLevel_ERROR,
						knetpb.RecordCode_DATA,
						fmt.Sprintf("File assembly %q failed: %s", fileName, err),
					))
				} else {
					kn.settings.Journal.Write(Proto.NewRecord(
						"KNet",
						knetpb.RecordLevel_INFO,
						knetpb.RecordCode_DATA,
						fmt.Sprintf("File accepted and verified: %s", fileName),
					))
				}
			}
		}
	}()

	return nil
}

//endregion
