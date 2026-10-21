import type { Participant, RhythmEvent } from '../../types/api';

export interface MeetingParticipant extends Participant { organizer?: boolean }

// Keep the organizer in the display list without adding them to editable invitees.
export function meetingParticipants(event: RhythmEvent, people: Participant[] = []): MeetingParticipant[] {
  const enrich = (participant: Participant): Participant => {
    const matches = people.filter(person => participant.id ? person.id === participant.id :
      participant.email ? person.email?.toLowerCase() === participant.email.toLowerCase() :
        person.name.trim().toLocaleLowerCase() === participant.name.trim().toLocaleLowerCase());
    const person = matches.length === 1 ? matches[0] : undefined;
    return { ...participant, id: participant.id || person?.id, avatar_url: participant.avatar_url || person?.avatar_url };
  };
  const participants: MeetingParticipant[] = (event.participants || []).map(enrich);
  if (!event.organizer_name) return participants;
  const organizer = enrich({ id: event.organizer_id, name: event.organizer_name, avatar_url: event.organizer_avatar_url });
  const index = participants.findIndex(person => person.id && organizer.id ? person.id === organizer.id : person.name.trim().toLocaleLowerCase() === organizer.name.trim().toLocaleLowerCase());
  if (index >= 0) participants[index] = { ...participants[index], ...organizer, organizer: true };
  else participants.unshift({ ...organizer, organizer: true });
  return participants;
}

export function participantCountLabel(count: number) {
  const last = count % 10;
  const teen = count % 100 >= 11 && count % 100 <= 14;
  return `${count} ${!teen && last === 1 ? 'участник' : !teen && last >= 2 && last <= 4 ? 'участника' : 'участников'}`;
}
