import { ConnectionForm, type ConnectionFormProps } from './ConnectionForm';
export function ConnectForm(props: ConnectionFormProps) {
  return <main className="ux-home"><div className="ux-home-heading"><h1>作業を始める</h1><p>接続先を選んで SSH ターミナルを開きます。</p></div><ConnectionForm {...props} /></main>;
}
