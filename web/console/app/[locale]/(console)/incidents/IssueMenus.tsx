import * as Menu from "@radix-ui/react-dropdown-menu";
import { Check, ChevronDown, User } from "lucide-react";
import { useTranslations } from "next-intl";
import { isActive, nextStates, type Issue, type IssueStatus } from "./issuesModel";

type Props = {
  issue: Issue;
  me?: string;
  disabled: boolean;
  onState: (status: IssueStatus) => void;
  onOwner: (owner: string) => void;
};

/**
 * The issue's state and owner as two header menus. The state menu offers only the moves the hub accepts; a resolved or
 * closed issue offers Reopen (back to investigating). The owner menu takes the issue or clears its owner.
 */
export function IssueMenus({ issue, me, disabled, onState, onOwner }: Props) {
  const t = useTranslations("incidents");
  const state = t(`states.${issue.status}`);
  const owner = issue.assignee || t("detail.unassigned");
  const moveLabel = (to: IssueStatus) => (!isActive(issue) && to === "investigating" ? t("detail.reopen") : t(`states.${to}`));

  return (
    <div className="iu-acts">
      <Menu.Root>
        <Menu.Trigger className="iu-hbtn" disabled={disabled} aria-label={t("detail.stateMenu", { state })}>
          {state}
          <ChevronDown aria-hidden="true" strokeWidth={2} />
        </Menu.Trigger>
        <Menu.Portal>
          <Menu.Content className="lt-menu" align="end" sideOffset={6}>
            <Menu.Item className="lt-menu-item">
              {state}
              <span className="lt-menu-check"><Check aria-hidden="true" /></span>
            </Menu.Item>
            <Menu.Separator className="lt-menu-sep" />
            {nextStates(issue.status).map((to) => (
              <Menu.Item key={to} className="lt-menu-item" onSelect={() => onState(to)}>{moveLabel(to)}</Menu.Item>
            ))}
          </Menu.Content>
        </Menu.Portal>
      </Menu.Root>
      <Menu.Root>
        <Menu.Trigger className="iu-hbtn" disabled={disabled || (!me && !issue.assignee)} aria-label={t("detail.ownerMenu", { owner })}>
          <User aria-hidden="true" strokeWidth={2} />
          <span className="iu-own">{owner}</span>
          <ChevronDown aria-hidden="true" strokeWidth={2} />
        </Menu.Trigger>
        <Menu.Portal>
          <Menu.Content className="lt-menu" align="end" sideOffset={6}>
            {issue.assignee ? (
              <Menu.Item className="lt-menu-item">
                {issue.assignee}
                <span className="lt-menu-check"><Check aria-hidden="true" /></span>
              </Menu.Item>
            ) : null}
            {me && issue.assignee !== me ? (
              <Menu.Item className="lt-menu-item" onSelect={() => onOwner(me)}>{t("detail.assignToMe")}</Menu.Item>
            ) : null}
            {issue.assignee ? (
              <>
                <Menu.Separator className="lt-menu-sep" />
                <Menu.Item className="lt-menu-item" onSelect={() => onOwner("")}>{t("detail.unassign")}</Menu.Item>
              </>
            ) : null}
          </Menu.Content>
        </Menu.Portal>
      </Menu.Root>
    </div>
  );
}
