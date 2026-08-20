export type CadCommandState = { enabled: boolean; visible: boolean; active: boolean };
export type CadCommandInvocation = { continuous?: boolean };

export interface CadCommand {
  readonly id: string;
  execute(invocation?: CadCommandInvocation): void | Promise<void>;
  isEnabled?(): boolean;
  isVisible?(): boolean;
  isActive?(): boolean;
}

export class CommandRegistry {
  private readonly commands = new Map<string, CadCommand>();
  private readonly listeners = new Set<() => void>();
  private revision = 0;

  register(command: CadCommand): () => void {
    this.commands.set(command.id, command);
    this.notifyStateChanged();
    return () => {
      if (this.commands.get(command.id) === command) {
        this.commands.delete(command.id);
        this.notifyStateChanged();
      }
    };
  }

  has(commandID: string): boolean { return this.commands.has(commandID); }

  state(commandID: string): CadCommandState {
    const command = this.commands.get(commandID);
    if (!command) return { enabled: false, visible: false, active: false };
    return { enabled: command.isEnabled?.() ?? true, visible: command.isVisible?.() ?? true, active: command.isActive?.() ?? false };
  }

  async execute(commandID: string, invocation?: CadCommandInvocation): Promise<boolean> {
    const command = this.commands.get(commandID);
    const state = this.state(commandID);
    if (!command || !state.visible || !state.enabled) return false;
    await command.execute(invocation);
    this.notifyStateChanged();
    return true;
  }

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };
  getSnapshot = (): number => this.revision;

  notifyStateChanged(): void {
    this.revision += 1;
    for (const listener of this.listeners) listener();
  }
}
