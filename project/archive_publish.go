package project

import (
	"fmt"
	"os"
)

func publishExtractedShow(temporary, root string) error {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		if err := os.Rename(temporary, root); err != nil {
			return fmt.Errorf("publish extracted show: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect extracted show cache: %w", err)
	}
	backup := root + ".previous"
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("remove old extraction backup: %w", err)
	}
	if err := os.Rename(root, backup); err != nil {
		return fmt.Errorf("preserve extracted show cache: %w", err)
	}
	if err := os.Rename(temporary, root); err != nil {
		_ = os.Rename(backup, root)
		return fmt.Errorf("publish extracted show: %w", err)
	}
	_ = os.RemoveAll(backup)
	return nil
}
