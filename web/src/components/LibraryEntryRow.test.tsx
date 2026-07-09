import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { LibraryEntryRow } from "./LibraryEntryRow";

afterEach(cleanup);

describe("LibraryEntryRow", () => {
  it("renders the text as a clickable button", () => {
    render(<LibraryEntryRow text="always write tests" onSave={vi.fn()} />);
    expect(screen.getByText("always write tests")).toBeInTheDocument();
  });

  it("edits in place: click to edit, Enter commits via onSave", async () => {
    const onSave = vi.fn();
    render(<LibraryEntryRow text="always write tests" onSave={onSave} />);

    await userEvent.click(screen.getByText("always write tests"));
    const input = screen.getByDisplayValue("always write tests");
    await userEvent.clear(input);
    await userEvent.type(input, "no coverage theater{Enter}");

    expect(onSave).toHaveBeenCalledWith("no coverage theater");
  });

  it("cancels with Escape, calling neither onSave nor onEmptyCommit", async () => {
    const onSave = vi.fn();
    const onEmptyCommit = vi.fn();
    render(<LibraryEntryRow text="always write tests" onSave={onSave} onEmptyCommit={onEmptyCommit} />);

    await userEvent.click(screen.getByText("always write tests"));
    const input = screen.getByDisplayValue("always write tests");
    await userEvent.type(input, " more{Escape}");

    expect(onSave).not.toHaveBeenCalled();
    expect(onEmptyCommit).not.toHaveBeenCalled();
    expect(screen.getByText("always write tests")).toBeInTheDocument();
  });

  it("does not call onSave when the committed text is unchanged", async () => {
    const onSave = vi.fn();
    render(<LibraryEntryRow text="always write tests" onSave={onSave} />);

    await userEvent.click(screen.getByText("always write tests"));
    await userEvent.keyboard("{Enter}");

    expect(onSave).not.toHaveBeenCalled();
  });

  it("calls onEmptyCommit, not onSave, when committed as empty text", async () => {
    const onSave = vi.fn();
    const onEmptyCommit = vi.fn();
    render(<LibraryEntryRow text="always write tests" onSave={onSave} onEmptyCommit={onEmptyCommit} />);

    await userEvent.click(screen.getByText("always write tests"));
    const input = screen.getByDisplayValue("always write tests");
    await userEvent.clear(input);
    await userEvent.keyboard("{Enter}");

    expect(onEmptyCommit).toHaveBeenCalled();
    expect(onSave).not.toHaveBeenCalled();
  });

  it("silently reverts on empty commit when onEmptyCommit is not provided", async () => {
    const onSave = vi.fn();
    render(<LibraryEntryRow text="always write tests" onSave={onSave} />);

    await userEvent.click(screen.getByText("always write tests"));
    const input = screen.getByDisplayValue("always write tests");
    await userEvent.clear(input);
    await userEvent.keyboard("{Enter}");

    expect(onSave).not.toHaveBeenCalled();
    expect(screen.getByText("always write tests")).toBeInTheDocument();
  });
});
