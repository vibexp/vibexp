package models

import "testing"

func TestDefaultProjectRequest(t *testing.T) {
	req := DefaultProjectRequest()

	if req.Name != "Project 1" {
		t.Errorf("expected Name %q, got %q", "Project 1", req.Name)
	}
	if req.Slug != "project-1" {
		t.Errorf("expected Slug %q, got %q", "project-1", req.Slug)
	}
	if req.Description != "Your first project - rename or customize as needed" {
		t.Errorf("unexpected Description %q", req.Description)
	}
	if req.GitURL != "" {
		t.Errorf("expected empty GitURL, got %q", req.GitURL)
	}
	if req.Homepage != "" {
		t.Errorf("expected empty Homepage, got %q", req.Homepage)
	}
	if req.TeamID != nil {
		t.Errorf("expected nil TeamID, got %v", req.TeamID)
	}
}
