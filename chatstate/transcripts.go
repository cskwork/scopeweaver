package chatstate

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Autumn-27/norma/llm"
	"github.com/Autumn-27/norma/transcript"
)

type TranscriptRevision struct {
	ID              string `json:"id"`
	SessionKey      string `json:"session_key"`
	TranscriptID    string `json:"transcript_id"`
	EndRecordUUID   string `json:"end_record_uuid"`
	SHA256          string `json:"sha256"`
	HistoryRevision int64  `json:"history_revision,string"`
	// The checksum protects the prepared prefix. Normal turns append after it.
	ByteLength int64  `json:"byte_length"`
	Directory  string `json:"-"` // preparation-only authority; never accepted from HTTP
}

func recordID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}
func PrepareTranscript(ctx context.Context, store *transcript.Store, session SessionRef, records []transcript.Record) (out TranscriptRevision, err error) {
	if err = session.Validate(); err != nil {
		return
	}
	if store == nil {
		return out, fmt.Errorf("transcript store unavailable")
	}
	if err = ctx.Err(); err != nil {
		return
	}
	out = TranscriptRevision{ID: recordID(), SessionKey: session.Key()}
	out.TranscriptID = "chat-rev-" + out.ID
	path := store.MainPath(out.TranscriptID)
	out.Directory = filepath.Dir(path)
	normalized := make([]transcript.Record, len(records))
	seen := map[string]bool{}
	parent := ""
	for i, record := range records {
		if record.UUID == "" {
			record.UUID = recordID()
		}
		if seen[record.UUID] {
			return out, ErrAmbiguousHistory
		}
		seen[record.UUID] = true
		record.SessionID = out.TranscriptID
		record.ParentUUID = parent
		record.AgentID = ""
		record.IsSidechain = false
		if record.Timestamp == "" {
			record.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
		}
		normalized[i] = record
		parent = record.UUID
	}
	if err = ValidateChatRecords(normalized); err != nil {
		return
	}
	out.EndRecordUUID = parent
	var data bytes.Buffer
	encoder := json.NewEncoder(&data)
	for _, r := range normalized {
		if err = encoder.Encode(r); err != nil {
			return
		}
	}
	out.ByteLength = int64(data.Len())
	sum := sha256.Sum256(data.Bytes())
	out.SHA256 = hex.EncodeToString(sum[:])
	if err = os.MkdirAll(out.Directory, 0700); err != nil {
		return
	}
	temp := path + ".preparing"
	file, err := os.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return out, err
	}
	defer func() {
		file.Close()
		if err != nil {
			os.Remove(temp)
		}
	}()
	if _, err = file.Write(data.Bytes()); err != nil {
		return
	}
	if err = file.Sync(); err != nil {
		return
	}
	if err = file.Close(); err != nil {
		return
	}
	if err = ctx.Err(); err != nil {
		return
	}
	if err = os.Rename(temp, path); err != nil {
		return
	}
	manifest, err := json.Marshal(out)
	if err != nil {
		return out, err
	}
	if err = writeChatManifest(path+".manifest.json", manifest); err != nil {
		return
	}
	if err = syncChatDirectory(out.Directory); err != nil {
		return
	}
	err = ValidateTranscriptRevision(store, out)
	return
}
func writeChatManifest(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return err
	}
	return f.Sync()
}
func syncChatDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
func ValidateTranscriptRevision(store *transcript.Store, revision TranscriptRevision) error {
	if store == nil || revision.ID == "" || revision.TranscriptID != "chat-rev-"+revision.ID || strings.ContainsAny(revision.ID, "/\\.") || revision.ByteLength < 0 {
		return fmt.Errorf("invalid transcript manifest")
	}
	path := store.MainPath(revision.TranscriptID)
	manifestData, err := os.ReadFile(path + ".manifest.json")
	if err != nil {
		return err
	}
	var manifest TranscriptRevision
	if err = json.Unmarshal(manifestData, &manifest); err != nil {
		return err
	}
	if manifest.ID != revision.ID || manifest.TranscriptID != revision.TranscriptID || manifest.SessionKey != revision.SessionKey || manifest.SHA256 != revision.SHA256 || manifest.ByteLength != revision.ByteLength || manifest.EndRecordUUID != revision.EndRecordUUID {
		return fmt.Errorf("transcript manifest mismatch")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.CopyN(h, f, revision.ByteLength)
	if err != nil || n != revision.ByteLength {
		return fmt.Errorf("incomplete prepared transcript")
	}
	if hex.EncodeToString(h.Sum(nil)) != revision.SHA256 {
		return fmt.Errorf("transcript checksum mismatch")
	}
	records, err := store.Load(path)
	if err != nil {
		return err
	}
	if err = ValidateChatRecords(records); err != nil {
		return err
	}
	var prefix bytes.Buffer
	encoder := json.NewEncoder(&prefix)
	end := ""
	for _, r := range records {
		if int64(prefix.Len()) >= revision.ByteLength {
			break
		}
		if r.SessionID != revision.TranscriptID {
			return fmt.Errorf("transcript session mismatch")
		}
		if err = encoder.Encode(r); err != nil {
			return err
		}
		end = r.UUID
	}
	if int64(prefix.Len()) != revision.ByteLength || end != revision.EndRecordUUID {
		return fmt.Errorf("transcript boundary mismatch")
	}
	return nil
}

// ValidateChatRecords rejects orphan tool/reasoning chains before publication.
// Boundary snapshots form a new working set and may reuse earlier tool IDs.
func ValidateChatRecords(records []transcript.Record) error {
	pending := map[string]bool{}
	seen := map[string]bool{}
	parent := ""
	for _, r := range records {
		if r.UUID == "" || seen[r.UUID] {
			return ErrAmbiguousHistory
		}
		seen[r.UUID] = true
		if r.ParentUUID != "" && r.ParentUUID != parent {
			return fmt.Errorf("broken transcript parent chain")
		}
		parent = r.UUID
		if r.Type == "boundary" {
			if len(pending) > 0 {
				return fmt.Errorf("boundary splits tool exchange")
			}
			continue
		}
		if r.Type != "message" || r.Message == nil {
			return fmt.Errorf("invalid transcript record")
		}
		hadPending := len(pending) > 0
		if hadPending && r.Message.Role != llm.RoleUser {
			return fmt.Errorf("tool results must immediately follow the assistant tool call")
		}
		boundary := false
		for _, b := range r.Message.Content {
			if b.Type == llm.BlockCompactBoundary {
				boundary = true
			}
		}
		if boundary && len(pending) > 0 {
			return fmt.Errorf("boundary splits tool exchange")
		}
		for _, b := range r.Message.Content {
			switch b.Type {
			case llm.BlockToolUse:
				if r.Message.Role != llm.RoleAssistant {
					return fmt.Errorf("tool use has invalid role")
				}
				if b.ID == "" || pending[b.ID] {
					return fmt.Errorf("invalid tool use")
				}
				pending[b.ID] = true
			case llm.BlockToolResult:
				if r.Message.Role != llm.RoleUser {
					return fmt.Errorf("tool result has invalid role")
				}
				if !pending[b.ToolUseID] {
					return fmt.Errorf("orphan tool result %s", b.ToolUseID)
				}
				delete(pending, b.ToolUseID)
			}
		}
		if hadPending && len(pending) > 0 {
			return fmt.Errorf("incomplete tool result batch")
		}
	}
	if len(pending) > 0 {
		return fmt.Errorf("incomplete tool exchange")
	}
	return nil
}

// MapLegacyChatTurns uses complete chronological prompt values. It never guesses
// from summaries or from a transcript containing reconstructed boundary copies.
func MapLegacyChatTurns(records []transcript.Record, prompts []string) ([]string, error) {
	var candidates []transcript.Record
	for _, r := range records {
		if r.Type == "boundary" {
			return nil, ErrAmbiguousHistory
		}
		if r.Message == nil {
			continue
		}
		if _, ok := llm.ParseBoundaryMeta(*r.Message); ok {
			return nil, ErrAmbiguousHistory
		}
		if r.Message.Role == llm.RoleUser {
			tool := false
			for _, b := range r.Message.Content {
				if b.Type == llm.BlockToolResult {
					tool = true
				}
			}
			if !tool {
				candidates = append(candidates, r)
			}
		}
	}
	if len(candidates) != len(prompts) {
		return nil, ErrAmbiguousHistory
	}
	ids := make([]string, len(prompts))
	for i, prompt := range prompts {
		if candidates[i].UUID == "" || candidates[i].Message.Text() != prompt {
			return nil, ErrAmbiguousHistory
		}
		ids[i] = candidates[i].UUID
	}
	return ids, nil
}
