declare module 'asciinema-player' {
  interface PlayerOptions {
    fit?: 'width' | 'height' | 'both' | 'none';
    terminalFontSize?: string | number;
    autoPlay?: boolean;
    loop?: boolean;
    speed?: number;
    theme?: string;
  }

  interface PlayerInstance {
    dispose?: () => void;
  }

  export function create(
    src: string,
    container: HTMLElement,
    opts?: PlayerOptions,
  ): PlayerInstance;
}
