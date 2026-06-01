import { useRef } from 'react';

export interface PendingKey {
  basename: string;
  keyType: 'main' | 'jump';
}

interface KeyRequiredModalProps {
  pending: PendingKey[];
  onCancel: () => void;
  onKeyFileChange: (basename: string, keyType: 'main' | 'jump', e: React.ChangeEvent<HTMLInputElement>) => void;
}

/** Modal prompting the user to supply private key files missing from profiles. */
export function KeyRequiredModal({ pending, onCancel, onKeyFileChange }: KeyRequiredModalProps) {
  const inputRefs = useRef<Map<string, HTMLInputElement | null>>(new Map());

  return (
    <div
      className="cf-modal-backdrop"
      onClick={(e) => { if (e.target === e.currentTarget) onCancel(); }}
      onKeyDown={(e) => { if (e.key === 'Escape') onCancel(); }}
      tabIndex={-1}
      role="dialog"
      aria-modal="true"
      aria-label="Key files required"
    >
      <div className="cf-modal">
        <div className="cf-modal-header">
          <span className="cf-modal-title">Key files required</span>
          <button type="button" className="cf-modal-close" onClick={onCancel} aria-label="Close">✕</button>
        </div>
        <div className="cf-modal-body">
          {pending.map(({ basename, keyType }) => {
            const inputKey = `${keyType}:${basename}`;
            return (
              <div key={inputKey} className="cf-modal-key-row">
                <input
                  ref={(el) => inputRefs.current.set(inputKey, el)}
                  type="file"
                  style={{ display: 'none' }}
                  onChange={(e) => onKeyFileChange(basename, keyType, e)}
                />
                <span className="cf-modal-key-name">
                  {basename}
                  {keyType === 'jump' && <span className="cf-key-pick-jump"> (jump)</span>}
                </span>
                <button
                  type="button"
                  className="cf-key-pick-select-btn"
                  onClick={() => inputRefs.current.get(inputKey)?.click()}
                >
                  Select
                </button>
              </div>
            );
          })}
        </div>
        <div className="cf-modal-footer">
          <button type="button" className="cf-save-profile-cancel" onClick={onCancel}>Cancel</button>
        </div>
      </div>
    </div>
  );
}
