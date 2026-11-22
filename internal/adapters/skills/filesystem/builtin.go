package skills

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
)

const (
	builtinSkillName = "opera"
	builtinSource    = "builtin"
	builtinRoot      = "builtin/" + builtinSkillName
)

//go:embed builtin/opera
var builtinFiles embed.FS

func loadBuiltinSkill() (Skill, error) {
	data, err := readBuiltinResource("SKILL.md")
	if err != nil {
		return Skill{}, err
	}
	if len(data) > MaxSkillFileBytes {
		return Skill{}, fmt.Errorf("built-in SKILL.md exceeds %d byte limit", MaxSkillFileBytes)
	}
	skill, err := parseSkill(data, builtinSkillName, builtinSource)
	if err != nil {
		return Skill{}, err
	}
	digest := sha256.New()
	err = fs.WalkDir(builtinFiles, builtinRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative := path[len(builtinRoot)+1:]
		content, err := readBuiltinResource(relative)
		if err != nil {
			return err
		}
		skill.PackageBytes += int64(len(content))
		_, _ = fmt.Fprintf(digest, "%s\x00%x\x00", relative, sha256.Sum256(content))
		return nil
	})
	if err != nil {
		return Skill{}, err
	}
	skill.Digest = hex.EncodeToString(digest.Sum(nil))
	return skill, nil
}

func readBuiltinResource(relative string) ([]byte, error) {
	if relative == "." || !fs.ValidPath(relative) {
		return nil, fmt.Errorf("invalid built-in skill resource path %q", relative)
	}
	data, err := builtinFiles.ReadFile(builtinRoot + "/" + relative)
	if err != nil {
		return nil, fmt.Errorf("read built-in skill resource: %w", err)
	}
	if len(data) > MaxResourceFileBytes {
		return nil, fmt.Errorf("built-in skill resource exceeds %d byte limit", MaxResourceFileBytes)
	}
	return data, nil
}
