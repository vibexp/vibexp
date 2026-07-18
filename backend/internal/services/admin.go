package services

import (
	"context"

	"github.com/vibexp/vibexp/internal/models"
	"github.com/vibexp/vibexp/internal/repositories"
)

// AdminServiceInterface exposes instance-level administrative reads. It backs
// the /api/v1/admin surface (guarded by instance-admin auth at the transport
// layer) and is intentionally separate from the team-scoped services and from
// the machine-only BackofficeService.
type AdminServiceInterface interface {
	// GetInstanceCounts returns instance-wide totals for the top-level entities.
	GetInstanceCounts(ctx context.Context) (models.InstanceCounts, error)
}

// AdminService implements AdminServiceInterface.
type AdminService struct {
	adminRepo repositories.AdminRepository
}

// NewAdminService creates a new AdminService.
func NewAdminService(adminRepo repositories.AdminRepository) AdminServiceInterface {
	return &AdminService{
		adminRepo: adminRepo,
	}
}

// GetInstanceCounts returns instance-wide entity totals from the repository.
func (s *AdminService) GetInstanceCounts(ctx context.Context) (models.InstanceCounts, error) {
	return s.adminRepo.GetInstanceCounts(ctx)
}
