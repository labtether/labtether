"use client";

import { Card } from "../../../components/ui/Card";
import { Button } from "../../../components/ui/Button";
import { Input, Select } from "../../../components/ui/Input";
import { TimezoneInput } from "../../../components/ui/TimezoneInput";

type NodesCreateGroupModalProps = {
  open: boolean;
  creatingGroup: boolean;
  createGroupError: string;
  newGroupName: string;
  newGroupCode: string;
  newGroupTimezone: string;
  newGroupLocation: string;
  newGroupStatus: string;
  onClose: () => void;
  onNewGroupNameChange: (value: string) => void;
  onNewGroupCodeChange: (value: string) => void;
  onNewGroupTimezoneChange: (value: string) => void;
  onNewGroupLocationChange: (value: string) => void;
  onNewGroupStatusChange: (value: string) => void;
  onSubmit: () => void;
};

export function NodesCreateGroupModal({
  open,
  creatingGroup,
  createGroupError,
  newGroupName,
  newGroupCode,
  newGroupTimezone,
  newGroupLocation,
  newGroupStatus,
  onClose,
  onNewGroupNameChange,
  onNewGroupCodeChange,
  onNewGroupTimezoneChange,
  onNewGroupLocationChange,
  onNewGroupStatusChange,
  onSubmit,
}: NodesCreateGroupModalProps) {
  if (!open) {
    return null;
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
      onClick={onClose}
    >
      <div onClick={(event) => { event.stopPropagation(); }}>
        <Card className="w-[34rem] max-w-[92vw] space-y-4">
          <h3 className="text-sm font-medium text-[var(--text)]">Create Group</h3>
          <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
            <label className="space-y-1">
              <span className="text-[10px] text-[var(--muted)]">Name</span>
              <Input
                value={newGroupName}
                onChange={(event) => onNewGroupNameChange(event.target.value)}
                placeholder="Home Lab"
                disabled={creatingGroup}
              />
            </label>
            <label className="space-y-1">
              <span className="text-[10px] text-[var(--muted)]">Code</span>
              <Input
                value={newGroupCode}
                onChange={(event) => onNewGroupCodeChange(event.target.value.toUpperCase())}
                placeholder="HOME"
                maxLength={24}
                disabled={creatingGroup}
              />
            </label>
            <label className="space-y-1">
              <span className="text-[10px] text-[var(--muted)]">Timezone (optional)</span>
              <TimezoneInput
                value={newGroupTimezone}
                onChange={onNewGroupTimezoneChange}
                placeholder="America/New_York"
                disabled={creatingGroup}
                ariaLabel="Group timezone"
              />
            </label>
            <label className="space-y-1">
              <span className="text-[10px] text-[var(--muted)]">Status</span>
              <Select
                value={newGroupStatus}
                onChange={(event) => onNewGroupStatusChange(event.target.value)}
                disabled={creatingGroup}
              >
                <option value="active">Active</option>
                <option value="inactive">Inactive</option>
                <option value="maintenance">Maintenance</option>
              </Select>
            </label>
            <label className="space-y-1 md:col-span-2">
              <span className="text-[10px] text-[var(--muted)]">Location (optional)</span>
              <Input
                value={newGroupLocation}
                onChange={(event) => onNewGroupLocationChange(event.target.value)}
                placeholder="Basement Rack"
                disabled={creatingGroup}
              />
            </label>
          </div>
          {createGroupError ? <p className="text-xs text-red-500">{createGroupError}</p> : null}
          <div className="flex items-center justify-end gap-2">
            <Button variant="secondary" onClick={onClose} disabled={creatingGroup}>
              Cancel
            </Button>
            <Button variant="primary" onClick={onSubmit} disabled={creatingGroup}>
              {creatingGroup ? "Creating..." : "Create Group"}
            </Button>
          </div>
        </Card>
      </div>
    </div>
  );
}
