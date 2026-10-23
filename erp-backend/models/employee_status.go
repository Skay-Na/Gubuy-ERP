package models

import "time"

const (
	EmployeeStatusActive      = "active"
	EmployeeStatusOffboarding = "offboarding"
	EmployeeStatusDeparted    = "departed"
)

// WasEmployedAt reports whether the employee was employed on the given local calendar date.
func (employee Employee) WasEmployedAt(at time.Time) bool {
	day := at.In(time.Local).Format("2006-01-02")
	start := employee.CreatedAt
	if employee.EntryDate != nil {
		start = *employee.EntryDate
	}
	if day < start.In(time.Local).Format("2006-01-02") {
		return false
	}
	end := employee.EmploymentEndDate()
	return end == nil || day <= end.In(time.Local).Format("2006-01-02")
}

// EmploymentEndDate returns the earliest recorded work end or soft-delete date.
func (employee Employee) EmploymentEndDate() *time.Time {
	end := employee.LastWorkingDate
	if employee.DeletedAt.Valid && (end == nil || employee.DeletedAt.Time.Before(*end)) {
		deletedAt := employee.DeletedAt.Time
		end = &deletedAt
	}
	return end
}

// CanUseEmployeeAccess keeps legacy blank statuses active while enforcing a recorded departure date.
func (employee Employee) CanUseEmployeeAccess(at time.Time) bool {
	status := employee.EmploymentStatus
	if status == "" {
		status = EmployeeStatusActive
	}
	if status == EmployeeStatusDeparted {
		return false
	}
	if status != EmployeeStatusActive && status != EmployeeStatusOffboarding {
		return false
	}
	if status == EmployeeStatusOffboarding && employee.LastWorkingDate != nil {
		return at.In(time.Local).Format("2006-01-02") <= employee.LastWorkingDate.In(time.Local).Format("2006-01-02")
	}
	return true
}
