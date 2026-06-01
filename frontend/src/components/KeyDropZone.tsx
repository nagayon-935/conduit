import { useState } from 'react';
import type { KeyInfo } from '../utils/form';

interface KeyDropZoneProps {
  keyName: string;
  keyLoaded: boolean;
  keyInfo?: KeyInfo | null;
  disabled: boolean;
  onSelectClick: () => void;
  onFileDrop: (file: File) => void;
}

/** Drag-and-drop / file-select zone for a private key, with type/passphrase badges. */
export function KeyDropZone({ keyName, keyLoaded, keyInfo, disabled, onSelectClick, onFileDrop }: KeyDropZoneProps) {
  const [over, setOver] = useState(false);
  return (
    <div
      className={`cf-key-dropzone${over ? ' cf-key-dropzone--over' : ''}${keyLoaded ? ' cf-key-dropzone--loaded' : ''}`}
      onDragOver={(e) => { e.preventDefault(); if (!disabled) setOver(true); }}
      onDragLeave={() => setOver(false)}
      onDrop={(e) => {
        e.preventDefault();
        setOver(false);
        if (disabled) return;
        const file = e.dataTransfer.files?.[0];
        if (file) onFileDrop(file);
      }}
    >
      {keyLoaded ? (
        <>
          <span className="cf-key-dropzone-icon">✓</span>
          <span className="cf-key-dropzone-name" title={keyName}>{keyName}</span>
          {keyInfo && (
            <span className="cf-key-type-badge" title={`Key type: ${keyInfo.type}`}>{keyInfo.type}</span>
          )}
          <button type="button" className="cf-key-dropzone-change" onClick={onSelectClick} disabled={disabled}>Change</button>
          {keyInfo?.hasPassphrase && (
            <span className="cf-key-passphrase-warning" title="This key is passphrase-protected. SSH will prompt for the passphrase on connection.">
              🔒 Encrypted
            </span>
          )}
        </>
      ) : (
        <>
          <span className="cf-key-dropzone-icon">🔑</span>
          {keyName
            ? <span className="cf-key-dropzone-hint"><span className="cf-key-dropzone-expected">{keyName}</span> — drop to load</span>
            : <span className="cf-key-dropzone-hint">Drop private key here</span>
          }
          <span className="cf-key-dropzone-sep">or</span>
          <button type="button" className="cf-key-dropzone-btn" onClick={onSelectClick} disabled={disabled}>Select file</button>
        </>
      )}
    </div>
  );
}
