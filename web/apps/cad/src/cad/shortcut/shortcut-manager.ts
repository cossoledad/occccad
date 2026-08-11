import { InputResult, type CadKeyboardEvent } from "../input/input-types";

export type ShortcutBinding = {
  key: string;
  command: string;
  ctrl?: boolean;
  shift?: boolean;
  alt?: boolean;
  meta?: boolean;
  primary?: boolean;
  allowInEditable?: boolean;
  allowRepeat?: boolean;
};

type ShortcutContext = { id: string; bindings: ShortcutBinding[]; token: symbol };

export class ShortcutManager {
  private readonly contexts: ShortcutContext[] = [];

  constructor(private readonly executeCommand: (commandID: string) => void | Promise<unknown>) {}

  pushContext(id: string, bindings: ShortcutBinding[]): () => void {
    const context = { id, bindings, token: Symbol(id) };
    this.contexts.push(context);
    return () => {
      const index = this.contexts.findIndex((candidate) => candidate.token === context.token);
      if (index >= 0) this.contexts.splice(index, 1);
    };
  }

  get contextIDs(): string[] { return this.contexts.map((context) => context.id); }

  keyDown(event: CadKeyboardEvent): InputResult {
    for (let index = this.contexts.length - 1; index >= 0; index -= 1) {
      const binding = this.contexts[index].bindings.find((candidate) => this.matches(candidate, event));
      if (!binding) continue;
      void this.executeCommand(binding.command);
      return InputResult.Consumed;
    }
    return InputResult.Ignored;
  }

  private matches(binding: ShortcutBinding, event: CadKeyboardEvent): boolean {
    if (event.editableTarget && !binding.allowInEditable) return false;
    if (event.repeat && !binding.allowRepeat) return false;
    const primary = event.state.modifiers.ctrl || event.state.modifiers.meta;
    return binding.key.toLowerCase() === event.key.toLowerCase() &&
      (binding.primary === undefined || binding.primary === primary) &&
      (binding.ctrl === undefined || binding.ctrl === event.state.modifiers.ctrl) &&
      (binding.shift === undefined || binding.shift === event.state.modifiers.shift) &&
      (binding.alt === undefined || binding.alt === event.state.modifiers.alt) &&
      (binding.meta === undefined || binding.meta === event.state.modifiers.meta);
  }
}
