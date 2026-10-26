import type { ChatResponse } from '../../types/api';

type Reply = Pick<ChatResponse, 'reply'> & Partial<Pick<ChatResponse, 'events' | 'candidates' | 'conflicts' | 'zoom_meetings' | 'scheduling'>>;

// The cards contain the full visible list; speech keeps every line of the
// server reply, including entries below the collapsed card list.
export function assistantDisplayText(response: Reply & { action?: string }): string {
  const text = assistantReplyText(response);
  if (['list', 'search'].includes(response.action || '') && response.events?.length && text.includes('\n')) return text.split('\n').filter(line => !/^«.*» — /.test(line)).join('\n');
  return text;
}

// Keep record references in the response data, and show their titles to the user.
export function assistantReplyText(response: Reply): string {
  const records = [...(response.events || []), ...(response.candidates || []), ...(response.conflicts || []), ...(response.scheduling?.meetings || [])];
  const titles = new Map(records.map(event => [event.id, event.display_title || event.title]));
  response.zoom_meetings?.forEach(meeting => { if (!titles.has(meeting.event_id)) titles.set(meeting.event_id, meeting.title); });
  return response.reply.replace(/(«[^»]*»|“[^”]*”|"[^"]*")|(^|[\s(,;:—–])#(\d+)([ \t]*)/g, (match, quoted: string | undefined, prefix: string | undefined, id: string | undefined, spaces: string, offset: number, text: string) => {
    if (quoted) return quoted;
    if (/^[«“"]/.test(text.slice(offset + match.length))) return prefix || '';
    const title = titles.get(Number(id));
    return title ? `${prefix || ''}«${title}»${spaces}` : match;
  });
}
