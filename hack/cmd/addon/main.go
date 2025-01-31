/*
TEST addon adapter.
This is an example of an addon adapter that lists files
and creates an application bucket for each. Error handling is
deliberately minimized to reduce code clutter.
*/
package main

import (
	"bytes"
	"errors"
	hub "github.com/konveyor/tackle2-hub/addon"
	"github.com/konveyor/tackle2-hub/api"
	"os"
	"os/exec"
	pathlib "path"
	"strings"
	"time"
)

var (
	// hub integration.
	addon = hub.Addon
	Log   = hub.Log
)

type SoftError = hub.SoftError

//
// main
func main() {
	addon.Run(func() (err error) {
		//
		// Get the addon data associated with the task.
		d := &Data{}
		_ = addon.DataWith(d)
		if err != nil {
			return
		}
		//
		// Get application.
		application, err := addon.Task.Application()
		if err != nil {
			return
		}
		//
		// Find files.
		paths, _ := find(d.Path, 25)
		//
		// List directory.
		err = listDir(d, application, paths)
		if err != nil {
			return
		}
		//
		// Set fact.
		application.Facts["Listed"] = true
		err = addon.Application.Update(application)
		if err != nil {
			return
		}
		//
		// Add tags.
		err = addTags(application, "LISTED", "TEST")
		if err != nil {
			return
		}
		return
	})
}

//
// listDir builds and populates the bucket.
func listDir(d *Data, application *api.Application, paths []string) (err error) {
	//
	// Task update: Update the task with total number of
	// items to be processed by the addon.
	addon.Total(len(paths))
	//
	// List directory.
	output := pathlib.Join(application.Bucket, "list")
	_ = os.RemoveAll(output)
	_ = os.MkdirAll(output, 0777)
	//
	// Write files.
	for _, p := range paths {
		var b []byte
		//
		// Read file.
		b, err = os.ReadFile(p)
		if err != nil {
			if errors.Is(err, os.ErrPermission) {
				continue
			}
			return
		}
		//
		// Task update: The current addon activity.
		target := pathlib.Join(
			output,
			pathlib.Base(p))
		addon.Activity("writing: %s", p)
		//
		// Write file.
		err = os.WriteFile(
			target,
			b,
			0666)
		if err != nil {
			return
		}
		time.Sleep(time.Second)
		//
		// Task update: Increment the number of completed
		// items processed by the addon.
		addon.Increment()
	}
	//
	// Build the index.
	err = buildIndex(output)
	if err != nil {
		return
	}
	//
	// Task update: update the current addon activity.
	addon.Activity("done")
	return
}

//
// Build index.html
func buildIndex(output string) (err error) {
	addon.Activity("Building index.")
	time.Sleep(time.Second)
	dir := output
	path := pathlib.Join(dir, "index.html")
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer func() {
		_ = f.Close()
	}()
	body := []string{"<ul>"}
	list, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, name := range list {
		body = append(
			body,
			"<li><a href=\""+name.Name()+"\">"+name.Name()+"</a>")
	}

	body = append(body, "</ul>")

	_, _ = f.WriteString(strings.Join(body, "\n"))

	return
}

//
// find files.
func find(path string, max int) (paths []string, err error) {
	Log.Info("Listing.", "path", path)
	cmd := exec.Command(
		"find",
		path,
		"-maxdepth",
		"1",
		"-type",
		"f",
		"-readable")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		Log.Info(stderr.String())
		return
	}

	paths = strings.Fields(stdout.String())
	if len(paths) > max {
		paths = paths[:max]
	}

	Log.Info("List found.", "paths", paths)

	return
}

//
// addTags ensure tags created and associated with application.
// Ensure tag exists and associated with the application.
func addTags(application *api.Application, names ...string) (err error) {
	addon.Activity("Adding tags: %v", names)
	appTags := appTags(application)
	//
	// Fetch
	tpMap, err := tpMap()
	if err != nil {
		return
	}
	tagMap, err := tagMap()
	if err != nil {
		return
	}
	//
	// Ensure type exists.
	wanted := api.TagType{
		Name: "DIRECTORY",
		Color: "#2b9af3",
		Rank: 3,
	}
	tp, found := tpMap[wanted.Name]
	if !found {
		tp = wanted
		err = addon.TagType.Create(&tp)
		if err == nil {
			tpMap[tp.Name] = tp
		} else {
			return
		}
	} else {
		if wanted.Rank != tp.Rank || wanted.Color != tp.Color {
			err = &SoftError{
				Reason: "Tag (TYPE) conflict detected.",
			}
			return
		}
	}
	//
	// Add tags.
	for _, name := range names {
		_, found := appTags[name]
		if found {
			continue
		}
		wanted := api.Tag{
			Name: name,
			TagType: api.Ref{
				ID: tp.ID,
			},
		}
		tg, found := tagMap[wanted.Name]
		if !found {
			tg = wanted
			err = addon.Tag.Create(&tg)
			if err != nil {
				return
			}
		} else {
			if wanted.TagType.ID != tg.TagType.ID {
				err = &SoftError{
					Reason: "Tag conflict detected.",
				}
				return
			}
		}
		addon.Activity("[TAG] Associated: %s.", tg.Name)
		application.Tags = append(
			application.Tags,
			api.Ref{
				ID: tg.ID,
			})
	}
	//
	// Update application.
	err = addon.Application.Update(application)
	return
}

//
// tagMap builds a map of tags by name.
func tagMap() (m map[string]api.Tag, err error) {
	list, err := addon.Tag.List()
	if err != nil {
		return
	}
	m = map[string]api.Tag{}
	for _, tag := range list {
		m[tag.Name] = tag
	}
	return
}

//
// tpMap builds a map of tag types by name.
func tpMap() (m map[string]api.TagType, err error) {
	list, err := addon.TagType.List()
	if err != nil {
		return
	}
	m = map[string]api.TagType{}
	for _, t := range list {
		m[t.Name] = t
	}
	return
}

//
// appTags builds map of associated tags.
func appTags(application *api.Application) (m map[string]uint) {
	m = map[string]uint{}
	for _, ref := range application.Tags {
		m[ref.Name] = ref.ID
	}
	return
}

//
// Data Addon input.
type Data struct {
	// Path to be listed.
	Path string `json:"path"`
}
