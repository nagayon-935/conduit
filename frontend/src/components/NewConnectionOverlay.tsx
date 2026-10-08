import { ConnectionForm, type ConnectionFormProps } from './ConnectionForm';
import { Dialog } from './Dialog';
export function NewConnectionOverlay({ onClose, ...props }: ConnectionFormProps & { onClose: () => void }) {
  return <Dialog title="新しい接続" onClose={onClose}><ConnectionForm {...props} /></Dialog>;
}
